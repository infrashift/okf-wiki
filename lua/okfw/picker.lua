-- snacks.nvim picker integration for okfw
-- Requires snacks.nvim to be installed

local M = {}

-- Badges for the states worth noticing before opening a concept. A stale,
-- deprecated or draft hit looked exactly like the current answer, and a
-- human-reviewed one like an unchecked note. Unverified and stable are the
-- common case and get no badge, so the list stays quiet.
local function concept_badges(entry)
    local badges = {}
    if entry.trust == "human-reviewed" then
        table.insert(badges, "reviewed")
    end
    if entry.status == "draft" or entry.status == "deprecated" then
        table.insert(badges, entry.status)
    end
    if entry.stale == true then
        table.insert(badges, "stale")
    end
    return badges
end

-- Format a concept entry for display
local function format_concept_display(entry)
    local title = entry.title or entry.id or ""
    local project = (entry.project and entry.project ~= "") and entry.project or ""

    local display = title
    if project ~= "" then
        display = display .. " [" .. project .. "]"
    end
    local badges = concept_badges(entry)
    if #badges > 0 then
        display = display .. " (" .. table.concat(badges, ", ") .. ")"
    end
    return display
end

-- Build the ordinal string for fuzzy matching
local function format_concept_ordinal(entry)
    local title = entry.title or ""
    local project = (entry.project and entry.project ~= "") and entry.project or ""
    local tags = (type(entry.tags) == "table") and entry.tags or {}
    return title .. " " .. project .. " " .. table.concat(tags, " ")
end

-- Pure, so exposed for the integration suite, which runs without snacks.
M._format_concept_display = format_concept_display

local has_snacks, Snacks = pcall(require, "snacks")
if not has_snacks then
    return M
end

-- Get the okfw config from the main module
local function get_config()
    local ok, okfw = pcall(require, "okfw")
    if ok and okfw.config then
        return okfw.config
    end
    return { bin = "okfw" }
end

-- Parse JSON search results
local function parse_results(json_str)
    local ok, results = pcall(vim.json.decode, json_str)
    if not ok or type(results) ~= "table" then
        return {}
    end
    return results
end

-- Build CLI args from opts (shared between search and live_search)
local function build_search_args(opts, query)
    local args = { "search", "--json" }

    if opts.project and opts.project ~= "" then
        table.insert(args, "--project")
        table.insert(args, opts.project)
    end
    if opts.type and opts.type ~= "" then
        table.insert(args, "--type")
        table.insert(args, opts.type)
    end
    if opts.tags and type(opts.tags) == "table" then
        for _, tag in ipairs(opts.tags) do
            table.insert(args, "--tag")
            table.insert(args, tag)
        end
    end
    if opts.kind and opts.kind ~= "" then
        table.insert(args, "--kind")
        table.insert(args, opts.kind)
    end
    -- OKF lifecycle status: draft | stable | deprecated
    if opts.status and opts.status ~= "" then
        table.insert(args, "--status")
        table.insert(args, opts.status)
    end
    if opts.stale then
        table.insert(args, "--stale")
    end

    if opts.limit then
        table.insert(args, "--limit")
        table.insert(args, tostring(opts.limit))
    end

    if query and query ~= "" then
        table.insert(args, query)
    end

    return args
end

-- Shared keymap actions for search pickers
local function preview_action(picker, item)
    if item and item.file then
        picker:close()
        local okfw = require("okfw")
        okfw.preview_note(item.file)
    end
end

local function insert_link_action(picker, item)
    if item and item.data then
        picker:close()
        local okfw = require("okfw")
        okfw.insert_link(item.data, false)
    end
end

local function insert_titled_link_action(picker, item)
    if item and item.data then
        picker:close()
        local okfw = require("okfw")
        okfw.insert_link(item.data, true)
    end
end

-- Common search picker keymaps
local function search_keys()
    return {
        ["<C-p>"] = { "preview_note", mode = { "n", "i" } },
        ["<C-l>"] = { "insert_link", mode = { "n", "i" } },
        ["<C-S-l>"] = { "insert_titled_link", mode = { "n", "i" } },
    }
end

-- Search concepts with optional filters
M.search = function(opts)
    opts = opts or {}
    local config = get_config()

    local args = build_search_args(opts, opts.query)
    if not opts.limit then
        table.insert(args, "--limit")
        table.insert(args, "50")
    end

    -- Execute search
    local Job = require("plenary.job")

    local job = Job:new({
        command = config.bin,
        args = args,
    })

    job:sync(10000)
    local results_json = table.concat(job:result(), "\n")

    local results = parse_results(results_json)

    if #results == 0 then
        vim.notify("No concepts found", vim.log.levels.INFO)
        return
    end

    -- Build items for snacks.picker
    local items = {}
    for idx, entry in ipairs(results) do
        table.insert(items, {
            idx = idx,
            text = format_concept_ordinal(entry),
            display = format_concept_display(entry),
            file = entry.file_path,
            data = entry,
        })
    end

    Snacks.picker.pick({
        source = "okfw",
        title = opts.prompt_title or "Concepts",
        items = items,
        format = function(item, _picker)
            local ret = {}
            table.insert(ret, { item.display or item.text })
            return ret
        end,
        preview = "file",
        confirm = function(picker, item)
            picker:close()
            if item and item.file then
                vim.cmd("edit " .. vim.fn.fnameescape(item.file))
            end
        end,
        actions = {
            preview_note = preview_action,
            insert_link = insert_link_action,
            insert_titled_link = insert_titled_link_action,
        },
        win = {
            input = {
                keys = search_keys(),
            },
        },
    })
end

-- Search with live query input
M.live_search = function(opts)
    opts = opts or {}
    local config = get_config()

    Snacks.picker.pick({
        source = "okfw_live",
        title = opts.prompt_title or "Search Concepts",
        live = true,
        -- The initial search string a live finder receives as ctx.filter.search:
        -- without it, `:OkfwSearch! auth` opened an empty prompt.
        search = opts.default_text or opts.query,
        finder = function(fopts, ctx)
            local query = ctx.filter.search or ""
            if query == "" then
                return {}
            end

            local args = build_search_args(opts, query)
            table.insert(args, "--limit")
            table.insert(args, "30")

            local Job = require("plenary.job")

            local job = Job:new({
                command = config.bin,
                args = args,
            })

            job:sync(5000)
            local results_json = table.concat(job:result(), "\n")

            local results = parse_results(results_json)
            local items = {}

            for idx, entry in ipairs(results) do
                table.insert(items, {
                    idx = idx,
                    text = format_concept_ordinal(entry),
                    display = format_concept_display(entry),
                    file = entry.file_path,
                    data = entry,
                })
            end

            return items
        end,
        format = function(item, _picker)
            local ret = {}
            table.insert(ret, { item.display or item.text })
            return ret
        end,
        preview = "file",
        confirm = function(picker, item)
            picker:close()
            if item and item.file then
                vim.cmd("edit " .. vim.fn.fnameescape(item.file))
            end
        end,
        actions = {
            preview_note = preview_action,
            insert_link = insert_link_action,
            insert_titled_link = insert_titled_link_action,
        },
        win = {
            input = {
                keys = search_keys(),
            },
        },
    })
end

-- Browse by project
M.projects = function(opts)
    M.search(vim.tbl_extend("force", opts or {}, {
        prompt_title = "Concepts by Project",
    }))
end

-- Insert link picker (dedicated function for linking)
M.insert_link = function(opts)
    opts = opts or {}
    local okfw = require("okfw")
    okfw.link_picker(opts)
end

return M
