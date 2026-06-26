-- okfw.nvim - Markdown-specific settings
-- This file is auto-loaded for markdown files

-- Only set up if okfw plugin is loaded
if not vim.g.loaded_okfw then
    return
end

-- Check if this looks like a concept (has frontmatter with id field)
local function is_concept()
    local lines = vim.api.nvim_buf_get_lines(0, 0, 10, false)
    if #lines < 2 then return false end
    if lines[1] ~= "---" then return false end

    for i = 2, math.min(10, #lines) do
        if lines[i]:match("^id:") then
            return true
        end
        if lines[i] == "---" then
            break
        end
    end
    return false
end

-- Get the structured kind from frontmatter (e.g. "todo", "note", "daily-note").
-- The `kind` field is omitted for plain notes, so an absent kind means "note".
local function get_kind()
    local lines = vim.api.nvim_buf_get_lines(0, 0, 15, false)
    if #lines < 2 then return "note" end
    if lines[1] ~= "---" then return "note" end

    for i = 2, math.min(15, #lines) do
        if lines[i] == "---" then
            break
        end
        local val = lines[i]:match("^kind:%s*(.+)$")
        if val then
            -- Strip surrounding quotes and whitespace
            val = vim.trim(val)
            val = val:gsub('^"(.*)"$', '%1'):gsub("^'(.*)'$", '%1')
            return val
        end
    end
    return "note"
end

-- Set up buffer-local settings for concept files
if is_concept() then
    -- Set omnifunc for tag completion
    vim.bo.omnifunc = "v:lua.require'okfw'.omnifunc"

    -- Auto-index on save
    vim.api.nvim_create_autocmd("BufWritePost", {
        buffer = 0,
        callback = function()
            local file_path = vim.fn.expand("%:p")
            require("okfw").index_file(file_path)
        end,
        desc = "Auto-index concept on save",
    })

    -- Buffer-local keymaps for concept files
    local opts = { buffer = true, silent = true }

    -- Tag completion in insert mode
    vim.keymap.set("i", "<C-x><C-t>", function()
        require("okfw").complete_tags()
    end, vim.tbl_extend("force", opts, { desc = "Complete okfw tags" }))

    -- Quick link insertion
    vim.keymap.set("n", "<localleader>l", function()
        require("okfw").link_picker()
    end, vim.tbl_extend("force", opts, { desc = "Insert concept link" }))

    vim.keymap.set("n", "<localleader>L", function()
        require("okfw").link_picker({ include_title = true })
    end, vim.tbl_extend("force", opts, { desc = "Insert concept link with title" }))

    -- Toggle backlinks panel
    vim.keymap.set("n", "<localleader>b", function()
        require("okfw").toggle_backlinks()
    end, vim.tbl_extend("force", opts, { desc = "Toggle backlinks" }))

    -- Add tags
    vim.keymap.set("n", "<localleader>a", function()
        vim.ui.input({ prompt = "Tags (space-separated): " }, function(input)
            if input and input ~= "" then
                require("okfw").add_tags(input)
            end
        end)
    end, vim.tbl_extend("force", opts, { desc = "Add tags" }))

    -- Validate frontmatter
    vim.keymap.set("n", "<localleader>v", function()
        require("okfw").validate()
    end, vim.tbl_extend("force", opts, { desc = "Validate frontmatter" }))

    -- Follow the markdown link under the cursor (resolves "/" to the bundle root)
    vim.keymap.set("n", "gf", function()
        require("okfw").follow_link()
    end, vim.tbl_extend("force", opts, { desc = "Follow okfw link" }))
    -- Let native gf also resolve bare relative .md paths
    vim.opt_local.suffixesadd:prepend(".md")

    local zkind = get_kind()

    -- Set project (note and todo kinds)
    if zkind == "note" or zkind == "todo" then
        vim.keymap.set("n", "<localleader>p", function()
            require("okfw").set_project()
        end, vim.tbl_extend("force", opts, { desc = "Set project" }))

        vim.keymap.set("n", "<localleader>t", function()
            vim.ui.select(
                { "tether", "untether" },
                { prompt = "Tether / Untether:" },
                function(choice)
                    if choice == "tether" then
                        require("okfw").tether_note()
                    elseif choice == "untether" then
                        require("okfw").untether_note()
                    end
                end
            )
        end, vim.tbl_extend("force", opts, { desc = "Tether / Untether" }))
    end

    -- Status picker for todo-kind concepts
    if zkind == "todo" then
        vim.keymap.set("n", "<localleader>s", function()
            vim.ui.select(
                { "open", "in_progress", "closed" },
                { prompt = "Set todo status:" },
                function(choice)
                    if choice then
                        require("okfw").set_status(choice)
                    end
                end
            )
        end, vim.tbl_extend("force", opts, { desc = "Set todo status" }))
    end
end
