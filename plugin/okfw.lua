-- okfw.nvim - Zettelkasten CLI integration for NeoVim
-- This file is auto-loaded by NeoVim when the plugin is installed

-- Prevent loading twice
if vim.g.loaded_okfw then
    return
end
vim.g.loaded_okfw = true

-- Minimum NeoVim version check
if vim.fn.has("nvim-0.9") ~= 1 then
    vim.notify("okfw.nvim requires NeoVim 0.9+", vim.log.levels.ERROR)
    return
end

-- User commands
local function create_commands()
    -- Daily notes
    vim.api.nvim_create_user_command("OkfwDaily", function(opts)
        local okfw = require("okfw")
        if opts.args == "yesterday" then
            okfw.daily({ date = "yesterday" })
        elseif opts.args ~= "" then
            okfw.daily({ date = opts.args })
        else
            okfw.daily()
        end
    end, {
        nargs = "?",
        complete = function()
            return { "yesterday" }
        end,
        desc = "Open or create daily note",
    })

    vim.api.nvim_create_user_command("OkfwDailyList", function(opts)
        local okfw = require("okfw")
        if opts.bang then
            okfw.daily_picker({ week = true })
        else
            okfw.daily_picker()
        end
    end, {
        bang = true,
        desc = "Browse daily notes (! for this week only)",
    })

    -- Note creation
    vim.api.nvim_create_user_command("OkfwNote", function(opts)
        local okfw = require("okfw")
        local args_parsed = {}

        -- Parse args
        local args_list = vim.split(opts.args, "%s+")
        local i = 1
        while i <= #args_list do
            local arg = args_list[i]
            if arg == "--category" and args_list[i + 1] then
                args_parsed.note_type = args_list[i + 1]
                i = i + 2
            elseif arg == "--project" and args_list[i + 1] then
                args_parsed.project = args_list[i + 1]
                i = i + 2
            else
                i = i + 1
            end
        end

        -- Default to untethered
        if not args_parsed.note_type then
            args_parsed.note_type = "untethered"
        end

        okfw.create_note(args_parsed)
    end, {
        nargs = "*",
        complete = function(_, line)
            local args = vim.split(line, "%s+")
            local last = args[#args] or ""
            local prev = args[#args - 1] or ""

            -- Complete values after flags
            if prev == "--category" then
                return { "untethered", "tethered" }
            end
            if prev == "--project" then
                return {}
            end

            -- Complete flags (filter out already-used ones)
            local used = {}
            for _, a in ipairs(args) do
                used[a] = true
            end
            local completions = {}
            if not used["--category"] then
                table.insert(completions, "--category")
            end
            if not used["--project"] then
                table.insert(completions, "--project")
            end
            return completions
        end,
        desc = "Create new concept (--category untethered/tethered --project NAME)",
    })

    vim.api.nvim_create_user_command("OkfwTemplate", function(opts)
        local okfw = require("okfw")
        local args_parsed = {}

        -- Parse args
        local args_list = vim.split(opts.args, "%s+")
        local i = 1
        while i <= #args_list do
            local arg = args_list[i]
            if arg == "--project" and args_list[i + 1] then
                args_parsed.project = args_list[i + 1]
                i = i + 2
            elseif not arg:match("^%-%-") then
                -- Assume it's the template name
                args_parsed.template = arg
                i = i + 1
            else
                i = i + 1
            end
        end

        if args_parsed.template then
            okfw.create_from_template(args_parsed)
        else
            okfw.template_picker()
        end
    end, {
        nargs = "*",
        complete = function(_, line)
            local args = vim.split(line, "%s+")
            local prev = args[#args - 1] or ""

            -- Complete project name (user types freely)
            if prev == "--project" then
                return {}
            end

            -- Check what's already been provided
            local has_template = false
            local has_project = false
            for _, a in ipairs(args) do
                if a == "--project" then
                    has_project = true
                elseif not a:match("^%-%-") and a ~= "" and a ~= "OkfwTemplate" then
                    has_template = true
                end
            end

            local completions = {}
            -- Offer template names if none selected yet
            if not has_template then
                local okfw = require("okfw")
                for _, tmpl in ipairs(okfw.templates) do
                    table.insert(completions, tmpl.name)
                end
            end
            if not has_project then
                table.insert(completions, "--project")
            end
            return completions
        end,
        desc = "Create note from template (--project NAME)",
    })

    -- Search and navigation
    vim.api.nvim_create_user_command("OkfwSearch", function(opts)
        local search_opts = {}
        local query_parts = {}

        -- Parse flags from args
        local args_list = vim.split(opts.args, "%s+")
        local i = 1
        while i <= #args_list do
            local arg = args_list[i]
            if arg == "--category" and args_list[i + 1] then
                search_opts.category = args_list[i + 1]
                i = i + 2
            elseif arg == "--project" and args_list[i + 1] then
                search_opts.project = args_list[i + 1]
                i = i + 2
            elseif arg == "--type" and args_list[i + 1] then
                search_opts.type = args_list[i + 1]
                i = i + 2
            elseif arg == "--status" and args_list[i + 1] then
                search_opts.status = args_list[i + 1]
                i = i + 2
            elseif arg == "--priority" and args_list[i + 1] then
                search_opts.priority = args_list[i + 1]
                i = i + 2
            elseif arg == "--due-before" and args_list[i + 1] then
                search_opts.due_before = args_list[i + 1]
                i = i + 2
            elseif arg == "--due-after" and args_list[i + 1] then
                search_opts.due_after = args_list[i + 1]
                i = i + 2
            elseif arg == "--tag" and args_list[i + 1] then
                search_opts.tags = search_opts.tags or {}
                table.insert(search_opts.tags, args_list[i + 1])
                i = i + 2
            elseif arg ~= "" then
                table.insert(query_parts, arg)
                i = i + 1
            else
                i = i + 1
            end
        end

        local query = table.concat(query_parts, " ")
        if query ~= "" then
            search_opts.query = query
            search_opts.default_text = query
        end

        local has_snacks = pcall(require, "snacks")
        if has_snacks then
            if opts.bang then
                require("okfw.picker").live_search(search_opts)
            else
                require("okfw.picker").search(search_opts)
            end
        else
            require("okfw").search(query, search_opts)
        end
    end, {
        nargs = "*",
        bang = true,
        complete = function(_, line)
            local args = vim.split(line, "%s+")
            local prev = args[#args - 1] or ""

            -- Complete values after flags
            if prev == "--category" then
                return { "untethered", "tethered" }
            end
            if prev == "--type" then
                return { "note", "todo", "daily-note", "issue" }
            end
            if prev == "--status" then
                return { "open", "in_progress", "closed" }
            end
            if prev == "--priority" then
                return { "high", "medium", "low" }
            end
            if prev == "--project" or prev == "--due-before" or prev == "--due-after" or prev == "--tag" then
                return {}
            end

            -- Complete flags (filter out already-used ones)
            local used = {}
            for _, a in ipairs(args) do
                used[a] = true
            end
            local completions = {}
            for _, flag in ipairs({ "--category", "--project", "--type", "--status", "--priority", "--due-before", "--due-after", "--tag" }) do
                if not used[flag] then
                    table.insert(completions, flag)
                end
            end
            return completions
        end,
        desc = "Search concepts (! for live search)",
    })

    -- Graph
    vim.api.nvim_create_user_command("OkfwGraph", function(opts)
        local okfw = require("okfw")
        local graph_opts = {}

        -- Parse args for --depth N, --start ID, --limit N, or plain number
        local args_list = vim.split(opts.args, "%s+")
        local i = 1
        while i <= #args_list do
            local arg = args_list[i]
            if arg == "--depth" and args_list[i + 1] then
                graph_opts.depth = tonumber(args_list[i + 1])
                i = i + 2
            elseif arg == "--start" and args_list[i + 1] then
                graph_opts.start = args_list[i + 1]
                i = i + 2
            elseif arg == "--limit" and args_list[i + 1] then
                graph_opts.limit = tonumber(args_list[i + 1])
                i = i + 2
            elseif tonumber(arg) then
                graph_opts.limit = tonumber(arg)
                i = i + 1
            elseif arg ~= "" then
                i = i + 1
            else
                i = i + 1
            end
        end

        okfw.graph(graph_opts)
    end, {
        nargs = "*",
        complete = function(_, line)
            local args = vim.split(line, "%s+")
            local prev = args[#args - 1] or ""

            -- No value completions for these flags
            if prev == "--depth" or prev == "--limit" or prev == "--start" then
                return {}
            end

            local used = {}
            for _, a in ipairs(args) do
                used[a] = true
            end
            local completions = {}
            for _, flag in ipairs({ "--depth", "--limit", "--start" }) do
                if not used[flag] then
                    table.insert(completions, flag)
                end
            end
            return completions
        end,
        desc = "Show graph tree (--depth N, --start ID, --limit N)",
    })

    -- Export
    vim.api.nvim_create_user_command("OkfwExport", function(opts)
        local okfw = require("okfw")
        local export_opts = {}

        -- Parse args for --depth N, --start ID, --limit N, or plain number
        local args_list = vim.split(opts.args, "%s+")
        local i = 1
        while i <= #args_list do
            local arg = args_list[i]
            if arg == "--depth" and args_list[i + 1] then
                export_opts.depth = tonumber(args_list[i + 1])
                i = i + 2
            elseif arg == "--start" and args_list[i + 1] then
                export_opts.start = args_list[i + 1]
                i = i + 2
            elseif arg == "--limit" and args_list[i + 1] then
                export_opts.limit = tonumber(args_list[i + 1])
                i = i + 2
            elseif tonumber(arg) then
                export_opts.limit = tonumber(arg)
                i = i + 1
            elseif arg ~= "" then
                i = i + 1
            else
                i = i + 1
            end
        end

        okfw.export(export_opts)
    end, {
        nargs = "*",
        complete = function(_, line)
            local args = vim.split(line, "%s+")
            local prev = args[#args - 1] or ""

            if prev == "--depth" or prev == "--limit" or prev == "--start" then
                return {}
            end

            local used = {}
            for _, a in ipairs(args) do
                used[a] = true
            end
            local completions = {}
            for _, flag in ipairs({ "--depth", "--limit", "--start" }) do
                if not used[flag] then
                    table.insert(completions, flag)
                end
            end
            return completions
        end,
        desc = "Export graph as portable markdown (--depth N, --start ID, --limit N)",
    })

    -- Index
    vim.api.nvim_create_user_command("OkfwIndex", function(opts)
        local okfw = require("okfw")
        local path = opts.args ~= "" and opts.args or nil
        okfw.index(path)
    end, {
        nargs = "?",
        complete = "dir",
        desc = "Index concepts",
    })

    -- Tags
    vim.api.nvim_create_user_command("OkfwRefreshTags", function()
        require("okfw").refresh_tags()
    end, {
        desc = "Refresh tag cache",
    })

    -- Todo commands
    vim.api.nvim_create_user_command("OkfwTodo", function(opts)
        local okfw = require("okfw")
        local args_parsed = {}

        -- Parse args for --due, --priority, --project
        local args_list = vim.split(opts.args, "%s+")
        local i = 1
        local title_parts = {}
        while i <= #args_list do
            local arg = args_list[i]
            if arg == "--due" and args_list[i + 1] then
                args_parsed.due = args_list[i + 1]
                i = i + 2
            elseif arg == "--priority" and args_list[i + 1] then
                args_parsed.priority = args_list[i + 1]
                i = i + 2
            elseif arg == "--project" and args_list[i + 1] then
                args_parsed.project = args_list[i + 1]
                i = i + 2
            else
                table.insert(title_parts, arg)
                i = i + 1
            end
        end

        if #title_parts > 0 then
            args_parsed.title = table.concat(title_parts, " ")
        end

        okfw.todo(args_parsed)
    end, {
        nargs = "*",
        complete = function(_, line)
            local args = vim.split(line, "%s+")
            local prev = args[#args - 1] or ""

            -- Complete values after flags
            if prev == "--priority" then
                return { "high", "medium", "low" }
            end
            if prev == "--due" or prev == "--project" then
                return {}
            end

            -- Complete flags (filter out already-used ones)
            local used = {}
            for _, a in ipairs(args) do
                used[a] = true
            end
            local completions = {}
            if not used["--due"] then
                table.insert(completions, "--due")
            end
            if not used["--priority"] then
                table.insert(completions, "--priority")
            end
            if not used["--project"] then
                table.insert(completions, "--project")
            end
            return completions
        end,
        desc = "Create new todo (--due DATE --priority high/medium/low --project NAME)",
    })

    vim.api.nvim_create_user_command("OkfwTodoList", function(opts)
        local okfw = require("okfw")
        local list_opts = {}

        if opts.args == "today" then
            list_opts.today = true
        elseif opts.args == "week" then
            list_opts.this_week = true
        elseif opts.args ~= "" then
            list_opts.project = opts.args
        end

        okfw.todo_list(list_opts)
    end, {
        nargs = "?",
        complete = function()
            return { "today", "week" }
        end,
        desc = "Generate todo list markdown",
    })
end

-- Create commands when plugin loads
create_commands()

-- Provide a convenient setup function that auto-configures everything
-- Users can call require("okfw").setup() for more control
vim.api.nvim_create_autocmd("User", {
    pattern = "VeryLazy",
    callback = function()
        -- If user hasn't called setup() yet, do a minimal setup
        local okfw = require("okfw")
        if not okfw.config then
            okfw.setup()
        end
    end,
})
