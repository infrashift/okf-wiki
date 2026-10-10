-- Integration test for okfw NeoVim plugin
-- Run with: make integration-test
-- (nvim --headless -u NONE -l test/integration_test.lua). Use -l, not
-- -c "luafile ...": under -c an uncaught Lua error leaves headless nvim waiting
-- for input instead of exiting, so a crash hangs CI rather than failing it.

local test_passed = true
local test_output = {}

local function log(msg)
    table.insert(test_output, msg)
    print(msg)
end

local function assert_eq(actual, expected, msg)
    if actual ~= expected then
        log(string.format("FAIL: %s - expected '%s', got '%s'", msg, tostring(expected), tostring(actual)))
        test_passed = false
    else
        log(string.format("PASS: %s", msg))
    end
end

local function assert_true(condition, msg)
    if not condition then
        log(string.format("FAIL: %s", msg))
        test_passed = false
    else
        log(string.format("PASS: %s", msg))
    end
end

-- Setup runtime path
local plugin_path = vim.fn.getcwd() .. "/lua"
local plenary_path = "/tmp/nvim-test-plugins/plenary.nvim/lua"
vim.opt.runtimepath:append(vim.fn.getcwd())
vim.opt.runtimepath:append("/tmp/nvim-test-plugins/plenary.nvim")

-- `nvim -u NONE` implies --noplugin, so plugin/okfw.lua is not sourced and the
-- :Okfw* user commands would be invisible to this suite. Source it explicitly
-- rather than switching to -u NORC, which would load whatever the developer
-- happens to have installed and make the run non-hermetic.
vim.cmd("runtime plugin/okfw.lua")

-- Wikis are per-project (.okfw/ discovered from the cwd), so the CLI tests
-- below are isolated inside a dedicated temp project — no HOME juggling needed.

log("=== okfw Plugin Integration Test ===")
log("")

-- Test 1: Plugin loads successfully
log("Test 1: Plugin loads")
local ok, okfw = pcall(require, "okfw")
assert_true(ok, "Plugin loads without error")
if not ok then
    log("Error: " .. tostring(okfw))
    vim.cmd("cq 1")
end

-- Test 2: Setup function works
log("")
log("Test 2: Setup function")
local okfw_bin = vim.fn.getcwd() .. "/okfw"
local setup_ok, setup_err = pcall(function()
    okfw.setup({ bin = okfw_bin })
end)
assert_true(setup_ok, "Setup completes without error")
if not setup_ok then
    log("Error: " .. tostring(setup_err))
end

-- Test 3: Config is set correctly
log("")
log("Test 3: Config values")
assert_eq(okfw.config.bin, okfw_bin, "Binary path is set correctly")

-- Test 4: Plenary.job is available
log("")
log("Test 4: Plenary dependency")
local plenary_ok, plenary_job = pcall(function()
    return require("plenary.job")
end)
assert_true(plenary_ok, "Plenary.job loads successfully")

-- CLI test setup: wikis are per-project (.okfw/), so CLI tests run inside a
-- dedicated temp project initialized with `okfw init`. In-process plugin
-- functions (insert_link, follow_link) discover the wiki from Neovim's cwd,
-- so the whole session moves into the test project.
local repo_root = vim.fn.getcwd()
local test_project = "/tmp/okfw-integration-project"
os.execute("rm -rf " .. test_project .. " && mkdir -p " .. test_project)
vim.cmd("cd " .. test_project)

-- Test 5: okfw init creates the project wiki (and repeat is a noop)
log("")
log("Test 5: okfw init")

if plenary_ok then
    local Job = require("plenary.job")

    local init_code = nil
    local init_job = Job:new({
        command = okfw_bin,
        args = { "init" },
        cwd = test_project,
        on_exit = function(_, return_val)
            init_code = return_val
        end,
    })
    init_job:sync(5000)
    assert_eq(init_code, 0, "okfw init exits with code 0")
    assert_eq(vim.fn.isdirectory(test_project .. "/.okfw"), 1, ".okfw directory created")

    local reinit_output = {}
    local reinit_code = nil
    local reinit_job = Job:new({
        command = okfw_bin,
        args = { "init" },
        cwd = test_project,
        on_stdout = function(_, data)
            table.insert(reinit_output, data)
        end,
        on_exit = function(_, return_val)
            reinit_code = return_val
        end,
    })
    reinit_job:sync(5000)
    assert_eq(reinit_code, 0, "repeat okfw init exits with code 0 (noop)")
    assert_true(table.concat(reinit_output, " "):find("already initialized") ~= nil,
        "repeat okfw init reports already initialized")
end

-- Test 5b: Integration test - actually run okfw via plenary.job
log("")
log("Test 5b: CLI integration via plenary.job")

if plenary_ok then
    local Job = require("plenary.job")
    local job_output = {}
    local job_code = nil

    local job = Job:new({
        command = okfw_bin,
        args = { "create", "Integration Test Note" },
        cwd = test_project,
        on_stdout = function(_, data)
            table.insert(job_output, data)
        end,
        on_exit = function(_, return_val)
            job_code = return_val
        end,
    })

    job:sync(5000) -- Wait up to 5 seconds

    assert_eq(job_code, 0, "okfw create exits with code 0")
    assert_true(#job_output > 0 or job_code == 0, "okfw create produces output or succeeds")

    if #job_output > 0 then
        log("  CLI output: " .. job_output[1])
    end
end

-- Test 6: Verify create_note function exists and is callable
log("")
log("Test 6: create_note function")
assert_true(type(okfw.create_note) == "function", "create_note is a function")

-- Test 7: Verify set_project function exists
log("")
log("Test 7: set_project function")
assert_true(type(okfw.set_project) == "function", "set_project is a function")

-- Test 8: Test set-project command via CLI
log("")
log("Test 8: CLI set-project command")
if plenary_ok then
    local Job = require("plenary.job")
    local function set_project(path)
        local out, code = {}, nil
        Job:new({
            command = okfw_bin,
            args = { "set-project", path, "new-project" },
            cwd = test_project,
            on_stdout = function(_, data) table.insert(out, data) end,
            on_exit = function(_, rv) code = rv end,
        }):sync(5000)
        return code, out
    end

    -- A concept inside the bundle.
    local inside = test_project .. "/.okfw/setproject-integration.md"
    vim.fn.writefile(vim.fn.readfile(repo_root .. "/cli/testdata/valid_concept_no_project.md"), inside)
    local code, out = set_project(inside)
    assert_eq(code, 0, "okfw set-project exits with code 0")
    assert_true(table.concat(vim.fn.readfile(inside), "\n"):find('project: "new-project"', 1, true) ~= nil,
        "set-project wrote the project")
    if #out > 0 then
        log("  CLI output: " .. out[1])
    end
    vim.fn.delete(inside)

    -- A file outside the bundle is refused rather than rewritten.
    local outside = vim.fn.tempname() .. ".md"
    vim.fn.writefile(vim.fn.readfile(repo_root .. "/cli/testdata/valid_concept_no_project.md"), outside)
    local before = table.concat(vim.fn.readfile(outside), "\n")
    code = set_project(outside)
    assert_true(code ~= 0, "set-project refuses a file outside the bundle")
    assert_eq(table.concat(vim.fn.readfile(outside), "\n"), before, "the outside file is untouched")
    vim.fn.delete(outside)
end

-- Test 9: Test search command via CLI
log("")
log("Test 9: CLI search command")
if plenary_ok then
    local Job = require("plenary.job")

    local search_output = {}
    local search_code = nil

    local search_job = Job:new({
        command = okfw_bin,
        args = { "search", "--json" },
        cwd = test_project,
        on_stdout = function(_, data)
            table.insert(search_output, data)
        end,
        on_exit = function(_, return_val)
            search_code = return_val
        end,
    })

    search_job:sync(5000)

    assert_eq(search_code, 0, "okfw search exits with code 0")

    -- Verify JSON output is parseable
    if #search_output > 0 then
        local json_str = table.concat(search_output, "")
        local ok, results = pcall(vim.json.decode, json_str)
        assert_true(ok, "okfw search JSON output is valid")
        if ok then
            log("  Found " .. #results .. " results")
        end
    end
end

-- Test 10: Verify search and index functions exist
log("")
log("Test 10: search and index functions")
assert_true(type(okfw.search) == "function", "search is a function")
assert_true(type(okfw.index) == "function", "index is a function")

-- Test 11: Verify graph function exists
log("")
log("Test 11: graph function")
assert_true(type(okfw.graph) == "function", "graph is a function")

-- Test 12: Verify preview functions exist
log("")
log("Test 12: preview functions")
assert_true(type(okfw.preview_note) == "function", "preview_note is a function")
assert_true(type(okfw.preview_by_id) == "function", "preview_by_id is a function")

-- Test 13: Verify link functions exist
log("")
log("Test 13: link functions")
assert_true(type(okfw.insert_link) == "function", "insert_link is a function")
assert_true(type(okfw.insert_link_prompt) == "function", "insert_link_prompt is a function")
assert_true(type(okfw.link_picker) == "function", "link_picker is a function")

-- Test 14: Verify tag completion functions exist
log("")
log("Test 14: tag completion functions")
assert_true(type(okfw.get_tags) == "function", "get_tags is a function")
assert_true(type(okfw.get_tags_sync) == "function", "get_tags_sync is a function")
assert_true(type(okfw.refresh_tags) == "function", "refresh_tags is a function")
assert_true(type(okfw.complete_tags) == "function", "complete_tags is a function")
assert_true(type(okfw.setup_tag_completion) == "function", "setup_tag_completion is a function")
assert_true(type(okfw.setup_cmp) == "function", "setup_cmp is a function")
assert_true(type(okfw.omnifunc) == "function", "omnifunc is a function")

-- Test 15: Verify backlinks functions exist
log("")
log("Test 15: backlinks functions")
assert_true(type(okfw.get_backlinks) == "function", "get_backlinks is a function")
assert_true(type(okfw.get_backlinks_sync) == "function", "get_backlinks_sync is a function")
assert_true(type(okfw.backlinks_panel) == "function", "backlinks_panel is a function")
assert_true(type(okfw.backlinks_split) == "function", "backlinks_split is a function")
assert_true(type(okfw.toggle_backlinks) == "function", "toggle_backlinks is a function")

-- Test 16: CLI backlinks command
log("")
log("Test 16: CLI backlinks command")
if plenary_ok then
    local Job = require("plenary.job")
    local backlinks_output = {}
    local backlinks_code = nil

    local backlinks_job = Job:new({
        command = okfw_bin,
        args = { "backlinks", "202602131045", "--json" },
        cwd = test_project,
        on_stdout = function(_, data)
            table.insert(backlinks_output, data)
        end,
        on_exit = function(_, return_val)
            backlinks_code = return_val
        end,
    })

    backlinks_job:sync(5000)

    assert_eq(backlinks_code, 0, "okfw backlinks exits with code 0")

    if #backlinks_output > 0 then
        local json_str = table.concat(backlinks_output, "")
        local ok, results = pcall(vim.json.decode, json_str)
        assert_true(ok, "okfw backlinks JSON output is valid")
        if ok and type(results) == "table" then
            log("  Found " .. #results .. " backlink(s)")
        end
    end
end

-- Test 17: Verify template functions exist
log("")
log("Test 17: template functions")
assert_true(type(okfw.templates) == "table", "templates is a table")
assert_true(type(okfw.get_template) == "function", "get_template is a function")
assert_true(type(okfw.create_from_template) == "function", "create_from_template is a function")
assert_true(type(okfw.template_picker) == "function", "template_picker is a function")

-- Test 18: CLI templates command
log("")
log("Test 18: CLI templates command")
if plenary_ok then
    local Job = require("plenary.job")
    local templates_output = {}
    local templates_code = nil

    local templates_job = Job:new({
        command = okfw_bin,
        args = { "templates" },
        cwd = test_project,
        on_stdout = function(_, data)
            table.insert(templates_output, data)
        end,
        on_exit = function(_, return_val)
            templates_code = return_val
        end,
    })

    templates_job:sync(5000)

    assert_eq(templates_code, 0, "okfw templates exits with code 0")
    assert_true(#templates_output > 0, "okfw templates produces output")

    -- Check that expected templates are listed
    local output_str = table.concat(templates_output, "\n")
    assert_true(output_str:find("meeting") ~= nil, "templates output contains 'meeting'")
    assert_true(output_str:find("user%-story") ~= nil, "templates output contains 'user-story'")
end

-- Test 19: CLI create with template
log("")
log("Test 19: CLI create with template")
if plenary_ok then
    local Job = require("plenary.job")
    local create_tmpl_output = {}
    local create_tmpl_code = nil

    local create_tmpl_job = Job:new({
        command = okfw_bin,
        args = { "create", "Template Test Meeting", "--template", "meeting" },
        cwd = test_project,
        on_stdout = function(_, data)
            table.insert(create_tmpl_output, data)
        end,
        on_exit = function(_, return_val)
            create_tmpl_code = return_val
        end,
    })

    create_tmpl_job:sync(5000)

    assert_eq(create_tmpl_code, 0, "okfw create --template meeting exits with code 0")

    if #create_tmpl_output > 0 then
        log("  CLI output: " .. create_tmpl_output[1])
    end
end

-- Test 20: Verify daily notes functions exist
log("")
log("Test 20: daily notes functions")
assert_true(type(okfw.daily) == "function", "daily is a function")
assert_true(type(okfw.list_daily) == "function", "list_daily is a function")
assert_true(type(okfw.list_daily_sync) == "function", "list_daily_sync is a function")
assert_true(type(okfw.daily_picker) == "function", "daily_picker is a function")

-- Test 21: CLI daily command
log("")
log("Test 21: CLI daily command")
if plenary_ok then
    local Job = require("plenary.job")
    local daily_output = {}
    local daily_code = nil

    local daily_job = Job:new({
        command = okfw_bin,
        args = { "daily" },
        cwd = test_project,
        on_stdout = function(_, data)
            table.insert(daily_output, data)
        end,
        on_exit = function(_, return_val)
            daily_code = return_val
        end,
    })

    daily_job:sync(5000)

    assert_eq(daily_code, 0, "okfw daily exits with code 0")
    assert_true(#daily_output > 0, "okfw daily produces output")

    if #daily_output > 0 then
        log("  CLI output: " .. daily_output[1])
    end
end

-- Test 22: CLI daily --list command
log("")
log("Test 22: CLI daily --list command")
if plenary_ok then
    local Job = require("plenary.job")
    local daily_list_output = {}
    local daily_list_code = nil

    local daily_list_job = Job:new({
        command = okfw_bin,
        args = { "daily", "--list", "--json" },
        cwd = test_project,
        on_stdout = function(_, data)
            table.insert(daily_list_output, data)
        end,
        on_exit = function(_, return_val)
            daily_list_code = return_val
        end,
    })

    daily_list_job:sync(5000)

    assert_eq(daily_list_code, 0, "okfw daily --list --json exits with code 0")

    if #daily_list_output > 0 then
        local json_str = table.concat(daily_list_output, "")
        local ok, results = pcall(vim.json.decode, json_str)
        assert_true(ok, "okfw daily --list JSON output is valid")
        if ok and type(results) == "table" then
            log("  Found " .. #results .. " daily note(s)")
        end
    end
end

-- ── OKF-era tests ─────────────────────────────────────────────────────────

local function run_okfw(args)
    local out, code = {}, nil
    local Job = require("plenary.job")
    local j = Job:new({
        command = okfw_bin,
        args = args,
        cwd = test_project,
        on_stdout = function(_, d) table.insert(out, d) end,
        on_exit = function(_, rv) code = rv end,
    })
    j:sync(8000)
    return code, table.concat(out, "\n")
end

-- Test 23: new link/navigation functions exist
log("")
log("Test 23: OKF link/navigation functions")
assert_true(type(okfw.bundle_root) == "function", "bundle_root is a function")
assert_true(type(okfw.bundle_rel) == "function", "bundle_rel is a function")
assert_true(type(okfw.follow_link) == "function", "follow_link is a function")

-- Test 24: CLI root prints the isolated bundle path
log("")
log("Test 24: CLI root")
local linked_target = nil
if plenary_ok then
    local code, out = run_okfw({ "root" })
    assert_eq(code, 0, "okfw root exits 0")
    assert_true(out:find("okfw", 1, true) ~= nil, "okfw root prints bundle path")
end

-- Test 25: create --path places a concept in a subdir with slug filename + OKF frontmatter
log("")
log("Test 25: create --path + frontmatter")
if plenary_ok then
    local code, out = run_okfw({ "create", "Link Target Note", "--path", "projects/itest" })
    assert_eq(code, 0, "okfw create --path exits 0")
    local path = out:match("Created: ([^\n]+)")
    assert_true(path ~= nil and path:find("projects/itest/", 1, true) ~= nil, "concept created under projects/itest")
    if path then
        path = vim.trim(path)
        linked_target = path
        assert_true(path:match("link%-target%-note%-%-%x+%.md$") ~= nil, "filename is slug--shortid.md")
        local fm = table.concat(vim.fn.readfile(path), "\n")
        assert_true(fm:find('type: "note"', 1, true) ~= nil, "frontmatter has OKF type")
        assert_true(fm:find("timestamp:", 1, true) ~= nil, "frontmatter has timestamp")
    end
end

-- Test 26: insert_link produces a vanilla markdown link
log("")
log("Test 26: insert_link (markdown)")
if linked_target then
    vim.cmd("enew")
    vim.api.nvim_buf_set_lines(0, 0, -1, false, { "" })
    vim.api.nvim_win_set_cursor(0, { 1, 0 })
    okfw.insert_link({ file_path = linked_target, title = "Link Target Note" }, true)
    local line = vim.api.nvim_get_current_line()
    assert_true(
        line:match("%[Link Target Note%]%(/projects/itest/link%-target%-note%-%-%x+%.md%)") ~= nil,
        "inserted a markdown link: " .. line
    )
end

-- Test 27: follow_link navigates a relative markdown link
-- (use a scratch dir outside the bundle so these fixtures aren't scanned)
log("")
log("Test 27: follow_link")
do
    local dir = vim.fn.tempname()
    vim.fn.mkdir(dir, "p")
    vim.fn.writefile({ "# B" }, dir .. "/follow-b--00000000.md")
    vim.fn.writefile({ "# A", "", "See [B](./follow-b--00000000.md)." }, dir .. "/follow-a--00000000.md")
    vim.cmd("edit " .. dir .. "/follow-a--00000000.md")
    vim.api.nvim_win_set_cursor(0, { 3, 7 })
    okfw.follow_link()
    assert_eq(vim.fn.expand("%:t"), "follow-b--00000000.md", "follow_link navigated to target")
end

-- Test 28: CLI validate --okf passes on a conformant bundle
log("")
log("Test 28: validate --okf")
if plenary_ok then
    local code, _ = run_okfw({ "validate", "--okf" })
    assert_eq(code, 0, "okfw validate --okf exits 0 on a conformant bundle")
end

-- Test 29: CLI index-md writes a root index.md with okf_version
log("")
log("Test 29: index-md")
if plenary_ok then
    local code, _ = run_okfw({ "index-md" })
    assert_eq(code, 0, "okfw index-md exits 0")
    local root_index = test_project .. "/.okfw/index.md"
    if vim.fn.filereadable(root_index) == 1 then
        local fm = table.concat(vim.fn.readfile(root_index), "\n")
        assert_true(fm:find("okf_version", 1, true) ~= nil, "root index.md declares okf_version")
    end
end

-- Test 30: CLI search --kind filter
log("")
log("Test 30: search --kind")
if plenary_ok then
    local code, out = run_okfw({ "search", "--kind", "note", "--json" })
    assert_eq(code, 0, "okfw search --kind note exits 0")
    local ok2, _ = pcall(vim.json.decode, out)
    assert_true(ok2, "search --kind JSON is valid")
end

-- Test 31: user commands are registered by plugin/okfw.lua
log("")
log("Test 31: user commands registered")
do
    local cmds = vim.api.nvim_get_commands({})
    for _, name in ipairs({
        "OkfwInit", "OkfwNote", "OkfwSearch", "OkfwGraph",
        "OkfwExport", "OkfwIndex", "OkfwDelete", "OkfwMove",
    }) do
        assert_true(cmds[name] ~= nil, name .. " is registered")
    end
end

-- Test 32: :OkfwMove relocates the concept, moves the buffer with it, and
-- repairs the inbound link in another concept.
log("")
log("Test 32: OkfwMove")
if plenary_ok then
    local code = run_okfw({ "create", "Movable Subject", "--path", "notes" })
    assert_eq(code, 0, "created a concept to move")
    local matches = vim.fn.glob(test_project .. "/.okfw/notes/movable-subject--*.md", false, true)
    assert_eq(#matches, 1, "found the created concept")

    if #matches == 1 then
        local target_rel = "/notes/" .. vim.fn.fnamemodify(matches[1], ":t")
        local lcode = run_okfw({ "create", "Mover Linker", "--body", "See [M](" .. target_rel .. ")." })
        assert_eq(lcode, 0, "created a linking concept")

        vim.cmd("edit " .. vim.fn.fnameescape(matches[1]))
        okfw.move_note(nil, "archive/")
        vim.wait(8000, function() return vim.fn.expand("%:p"):match("archive") ~= nil end)

        assert_true(vim.fn.expand("%:p"):match("archive") ~= nil, "buffer followed the concept to its new path")
        assert_true(vim.fn.filereadable(vim.fn.expand("%:p")) == 1, "the concept exists at its new path")
        assert_eq(vim.fn.filereadable(matches[1]), 0, "the old file is gone")

        -- The inbound link must now name the new location.
        local linkers = vim.fn.glob(test_project .. "/.okfw/mover-linker--*.md", false, true)
        if #linkers == 1 then
            local body = table.concat(vim.fn.readfile(linkers[1]), "\n")
            assert_true(body:find("/archive/", 1, true) ~= nil, "inbound link was retargeted to archive/")
        end
        -- Nothing may be left dangling.
        local vcode, vout = run_okfw({ "validate", "--okf" })
        assert_eq(vcode, 0, "bundle still conformant after the move")
        assert_true(vout:find("0 broken link", 1, true) ~= nil, "no broken links after the move")
    end
end

-- Test 33: :OkfwDelete confirms first, then removes the concept and its buffer.
log("")
log("Test 33: OkfwDelete")
if plenary_ok then
    local code = run_okfw({ "create", "Deletable Subject" })
    assert_eq(code, 0, "created a concept to delete")
    local matches = vim.fn.glob(test_project .. "/.okfw/deletable-subject--*.md", false, true)
    assert_eq(#matches, 1, "found the created concept")

    if #matches == 1 then
        local real_confirm = vim.fn.confirm
        vim.cmd("edit " .. vim.fn.fnameescape(matches[1]))

        -- Declining must leave the file alone: the destructive path is opt-in.
        vim.fn.confirm = function() return 1 end -- "&No"
        vim.cmd("OkfwDelete")
        vim.wait(600)
        assert_eq(vim.fn.filereadable(matches[1]), 1, "declining the prompt keeps the file")

        -- Confirming removes it.
        vim.fn.confirm = function() return 2 end -- "&Yes"
        vim.cmd("OkfwDelete")
        vim.wait(8000, function() return vim.fn.filereadable(matches[1]) == 0 end)
        assert_eq(vim.fn.filereadable(matches[1]), 0, "confirming the prompt deletes the file")

        vim.fn.confirm = real_confirm

        -- And search no longer finds it.
        local scode, sout = run_okfw({ "search", "Deletable", "--json" })
        assert_eq(scode, 0, "search runs after the delete")
        local ok2, hits = pcall(vim.json.decode, sout)
        assert_true(ok2 and type(hits) == "table", "search --json decodes")
        if ok2 and type(hits) == "table" then
            assert_eq(#hits, 0, "deleted concept is no longer found by search")
        end
    end
end

-- Test 34: saving a concept by hand refreshes the index.md listings. The save
-- hook used to run the removed `okfw index`, which failed on every :w.
log("")
log("Test 34: index_file refreshes listings after a hand edit")
if plenary_ok then
    local code = run_okfw({ "create", "Hand Edited" })
    assert_eq(code, 0, "created a concept to edit")
    local matches = vim.fn.glob(test_project .. "/.okfw/hand-edited--*.md", false, true)
    assert_eq(#matches, 1, "found the created concept")
    if #matches == 1 then
        local lines = vim.fn.readfile(matches[1])
        for i, l in ipairs(lines) do
            if l:match("^title:") then lines[i] = 'title: "Retitled By Hand"' end
        end
        vim.fn.writefile(lines, matches[1])
        local warned = false
        local real_notify = vim.notify
        vim.notify = function(msg, level)
            if level == vim.log.levels.WARN then warned = true end
            return real_notify(msg, level)
        end
        okfw.index_file(matches[1])
        local root_index = test_project .. "/.okfw/index.md"
        vim.wait(8000, function()
            return table.concat(vim.fn.readfile(root_index), "\n"):find("Retitled By Hand", 1, true) ~= nil
        end)
        vim.notify = real_notify
        assert_true(table.concat(vim.fn.readfile(root_index), "\n"):find("Retitled By Hand", 1, true) ~= nil,
            "root index.md lists the new title")
        assert_true(not warned, "the save hook reported no failure")
    end
end

-- Test 35: add_tags and validate drive the CLI. Both were bound to keymaps
-- after their functions had been deleted, so the keys threw.
log("")
log("Test 35: add_tags and validate")
if plenary_ok then
    local code = run_okfw({ "create", "Taggable" })
    assert_eq(code, 0, "created a concept to tag")
    local matches = vim.fn.glob(test_project .. "/.okfw/taggable--*.md", false, true)
    assert_eq(#matches, 1, "found the created concept")
    if #matches == 1 then
        okfw.add_tags("alpha, beta", matches[1])
        vim.wait(8000, function()
            return table.concat(vim.fn.readfile(matches[1]), "\n"):find('"beta"', 1, true) ~= nil
        end)
        local body = table.concat(vim.fn.readfile(matches[1]), "\n")
        assert_true(body:find('"alpha"', 1, true) ~= nil and body:find('"beta"', 1, true) ~= nil,
            "add_tags wrote both tags")

        local levels = {}
        local real_notify = vim.notify
        vim.notify = function(msg, level) table.insert(levels, level) end
        okfw.validate(matches[1])
        vim.wait(8000, function() return #levels > 0 end)
        assert_eq(levels[1], vim.log.levels.INFO, "validate reports a valid concept as valid")

        local broken = test_project .. "/.okfw/broken--0001.md"
        vim.fn.writefile({ "---", 'title: "Broken"', "---", "" }, broken)
        levels = {}
        okfw.validate(broken)
        vim.wait(8000, function() return #levels > 0 end)
        vim.notify = real_notify
        assert_eq(levels[1], vim.log.levels.ERROR, "validate reports an invalid concept as an error")
        vim.fn.delete(broken)
    end
end

-- Test 36: preview_by_id resolves a concept wherever it ranks. It used to run a
-- one-result search and look for the id inside it.
log("")
log("Test 36: resolve_concept")
if plenary_ok then
    local matches = vim.fn.glob(test_project .. "/.okfw/taggable--*.md", false, true)
    if #matches == 1 then
        local id
        for _, l in ipairs(vim.fn.readfile(matches[1], "", 20)) do
            id = id or l:match('^id:%s*"([^"]+)"')
        end
        okfw._bundle_roots = {}
        local real_cwd = vim.fn.getcwd()
        vim.cmd("cd " .. vim.fn.fnameescape(test_project))
        local want = vim.fn.resolve(matches[1])
        assert_eq(vim.fn.resolve(okfw.resolve_concept(id) or ""), want, "resolves by frontmatter id")
        local concept_id = vim.fn.fnamemodify(matches[1], ":t:r")
        assert_eq(vim.fn.resolve(okfw.resolve_concept(concept_id) or ""), want, "resolves by OKF concept ID")
        assert_eq(okfw.resolve_concept("019680f0-0000-7000-8000-000000000000"), nil, "unknown id resolves to nil")
        assert_eq(vim.fn.resolve(okfw.resolve_concept(id:sub(1, 13)) or ""), want, "resolves by a unique id prefix")
        vim.cmd("cd " .. vim.fn.fnameescape(real_cwd))
    end
end

-- Summary
log("")
log("=== Test Summary ===")
if test_passed then
    log("All tests PASSED")
    vim.cmd("qa!")
else
    log("Some tests FAILED")
    vim.cmd("cq 1")
end
