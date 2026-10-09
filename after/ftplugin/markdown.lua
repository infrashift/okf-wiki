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

-- Get the structured kind from frontmatter (e.g. "note", "daily-note", "issue").
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

    -- Refresh the index.md listings on save, so a title or description edited
    -- here shows up in them as it would after any okfw write.
    vim.api.nvim_create_autocmd("BufWritePost", {
        buffer = 0,
        callback = function()
            local file_path = vim.fn.expand("%:p")
            require("okfw").index_file(file_path)
        end,
        desc = "Refresh okfw index.md listings on save",
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

    -- Set project (plain notes only)
    if get_kind() == "note" then
        vim.keymap.set("n", "<localleader>p", function()
            require("okfw").set_project()
        end, vim.tbl_extend("force", opts, { desc = "Set project" }))
    end
end
