-- Integration test for okfw NeoVim plugin
-- Run with: nvim --headless -u NONE -c "luafile test/integration_test.lua"

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

-- Isolate the bundle so this test never touches the real ~/okfw and is
-- deterministic. okfw resolves its bundle root from $HOME by default.
local test_home = vim.fn.tempname()
vim.fn.mkdir(test_home, "p")
vim.env.HOME = test_home
local bundle = test_home .. "/okfw"

log("=== okfw Plugin Integration Test ===")
log("Isolated bundle: " .. bundle)
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
local setup_ok, setup_err = pcall(function()
    okfw.setup({ bin = "./okfw" })
end)
assert_true(setup_ok, "Setup completes without error")
if not setup_ok then
    log("Error: " .. tostring(setup_err))
end

-- Test 3: Config is set correctly
log("")
log("Test 3: Config values")
assert_eq(okfw.config.bin, "./okfw", "Binary path is set correctly")

-- Test 4: Plenary.job is available
log("")
log("Test 4: Plenary dependency")
local plenary_ok, plenary_job = pcall(function()
    return require("plenary.job")
end)
assert_true(plenary_ok, "Plenary.job loads successfully")

-- Test 5: Integration test - actually run okfw via plenary.job
log("")
log("Test 5: CLI integration via plenary.job")

if plenary_ok then
    local Job = require("plenary.job")
    local job_output = {}
    local job_code = nil

    local job = Job:new({
        command = "./okfw",
        args = { "create", "Integration Test Note", "--category", "untethered" },
        cwd = vim.fn.getcwd(),
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

-- Test 7: Verify tether_note function exists
log("")
log("Test 7: tether_note function")
assert_true(type(okfw.tether_note) == "function", "tether_note is a function")

-- Test 8: Verify set_project function exists
log("")
log("Test 8: set_project function")
assert_true(type(okfw.set_project) == "function", "set_project is a function")

-- Test 9: Test tether command via CLI
log("")
log("Test 9: CLI tether command")
if plenary_ok then
    local Job = require("plenary.job")

    -- First copy the test file
    os.execute("cp cli/testdata/valid_fleeting_no_project.md /tmp/test_tether_integration.md")

    local tether_output = {}
    local tether_code = nil

    local tether_job = Job:new({
        command = "./okfw",
        args = { "tether", "/tmp/test_tether_integration.md", "--project", "integration-test" },
        cwd = vim.fn.getcwd(),
        on_stdout = function(_, data)
            table.insert(tether_output, data)
        end,
        on_exit = function(_, return_val)
            tether_code = return_val
        end,
    })

    tether_job:sync(5000)

    assert_eq(tether_code, 0, "okfw tether exits with code 0")

    if #tether_output > 0 then
        log("  CLI output: " .. tether_output[1])
    end
end

-- Test 10: Test set-project command via CLI
log("")
log("Test 10: CLI set-project command")
if plenary_ok then
    local Job = require("plenary.job")

    local setproj_output = {}
    local setproj_code = nil

    local setproj_job = Job:new({
        command = "./okfw",
        args = { "set-project", "/tmp/test_tether_integration.md", "new-project" },
        cwd = vim.fn.getcwd(),
        on_stdout = function(_, data)
            table.insert(setproj_output, data)
        end,
        on_exit = function(_, return_val)
            setproj_code = return_val
        end,
    })

    setproj_job:sync(5000)

    assert_eq(setproj_code, 0, "okfw set-project exits with code 0")

    if #setproj_output > 0 then
        log("  CLI output: " .. setproj_output[1])
    end
end

-- Test 11: Test search command via CLI
log("")
log("Test 11: CLI search command")
if plenary_ok then
    local Job = require("plenary.job")

    local search_output = {}
    local search_code = nil

    local search_job = Job:new({
        command = "./okfw",
        args = { "search", "--json" },
        cwd = vim.fn.getcwd(),
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

-- Test 12: Verify search and index functions exist
log("")
log("Test 12: search and index functions")
assert_true(type(okfw.search) == "function", "search is a function")
assert_true(type(okfw.index) == "function", "index is a function")

-- Test 13: Verify graph function exists
log("")
log("Test 13: graph function")
assert_true(type(okfw.graph) == "function", "graph is a function")

-- Test 14: Verify preview functions exist
log("")
log("Test 14: preview functions")
assert_true(type(okfw.preview_note) == "function", "preview_note is a function")
assert_true(type(okfw.preview_by_id) == "function", "preview_by_id is a function")

-- Test 15: Verify link functions exist
log("")
log("Test 15: link functions")
assert_true(type(okfw.insert_link) == "function", "insert_link is a function")
assert_true(type(okfw.insert_link_prompt) == "function", "insert_link_prompt is a function")
assert_true(type(okfw.link_picker) == "function", "link_picker is a function")

-- Test 16: Verify tag completion functions exist
log("")
log("Test 16: tag completion functions")
assert_true(type(okfw.get_tags) == "function", "get_tags is a function")
assert_true(type(okfw.get_tags_sync) == "function", "get_tags_sync is a function")
assert_true(type(okfw.refresh_tags) == "function", "refresh_tags is a function")
assert_true(type(okfw.complete_tags) == "function", "complete_tags is a function")
assert_true(type(okfw.setup_tag_completion) == "function", "setup_tag_completion is a function")
assert_true(type(okfw.setup_cmp) == "function", "setup_cmp is a function")
assert_true(type(okfw.omnifunc) == "function", "omnifunc is a function")

-- Test 17: Verify backlinks functions exist
log("")
log("Test 17: backlinks functions")
assert_true(type(okfw.get_backlinks) == "function", "get_backlinks is a function")
assert_true(type(okfw.get_backlinks_sync) == "function", "get_backlinks_sync is a function")
assert_true(type(okfw.backlinks_panel) == "function", "backlinks_panel is a function")
assert_true(type(okfw.backlinks_split) == "function", "backlinks_split is a function")
assert_true(type(okfw.toggle_backlinks) == "function", "toggle_backlinks is a function")

-- Test 18: CLI backlinks command
log("")
log("Test 18: CLI backlinks command")
if plenary_ok then
    local Job = require("plenary.job")
    local backlinks_output = {}
    local backlinks_code = nil

    local backlinks_job = Job:new({
        command = "./okfw",
        args = { "backlinks", "202602131045", "--json" },
        cwd = vim.fn.getcwd(),
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

-- Test 19: Verify template functions exist
log("")
log("Test 19: template functions")
assert_true(type(okfw.templates) == "table", "templates is a table")
assert_true(type(okfw.get_template) == "function", "get_template is a function")
assert_true(type(okfw.create_from_template) == "function", "create_from_template is a function")
assert_true(type(okfw.template_picker) == "function", "template_picker is a function")

-- Test 20: CLI templates command
log("")
log("Test 20: CLI templates command")
if plenary_ok then
    local Job = require("plenary.job")
    local templates_output = {}
    local templates_code = nil

    local templates_job = Job:new({
        command = "./okfw",
        args = { "templates" },
        cwd = vim.fn.getcwd(),
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

-- Test 21: CLI create with template
log("")
log("Test 21: CLI create with template")
if plenary_ok then
    local Job = require("plenary.job")
    local create_tmpl_output = {}
    local create_tmpl_code = nil

    local create_tmpl_job = Job:new({
        command = "./okfw",
        args = { "create", "Template Test Meeting", "--template", "meeting" },
        cwd = vim.fn.getcwd(),
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

-- Test 22: Verify daily notes functions exist
log("")
log("Test 22: daily notes functions")
assert_true(type(okfw.daily) == "function", "daily is a function")
assert_true(type(okfw.list_daily) == "function", "list_daily is a function")
assert_true(type(okfw.list_daily_sync) == "function", "list_daily_sync is a function")
assert_true(type(okfw.daily_picker) == "function", "daily_picker is a function")

-- Test 23: CLI daily command
log("")
log("Test 23: CLI daily command")
if plenary_ok then
    local Job = require("plenary.job")
    local daily_output = {}
    local daily_code = nil

    local daily_job = Job:new({
        command = "./okfw",
        args = { "daily" },
        cwd = vim.fn.getcwd(),
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

-- Test 24: CLI daily --list command
log("")
log("Test 24: CLI daily --list command")
if plenary_ok then
    local Job = require("plenary.job")
    local daily_list_output = {}
    local daily_list_code = nil

    local daily_list_job = Job:new({
        command = "./okfw",
        args = { "daily", "--list", "--json" },
        cwd = vim.fn.getcwd(),
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
        command = "./okfw",
        args = args,
        cwd = vim.fn.getcwd(),
        on_stdout = function(_, d) table.insert(out, d) end,
        on_exit = function(_, rv) code = rv end,
    })
    j:sync(8000)
    return code, table.concat(out, "\n")
end

-- Test 25: new link/navigation functions exist
log("")
log("Test 25: OKF link/navigation functions")
assert_true(type(okfw.bundle_root) == "function", "bundle_root is a function")
assert_true(type(okfw.bundle_rel) == "function", "bundle_rel is a function")
assert_true(type(okfw.follow_link) == "function", "follow_link is a function")

-- Test 26: CLI root prints the isolated bundle path
log("")
log("Test 26: CLI root")
local linked_target = nil
if plenary_ok then
    local code, out = run_okfw({ "root" })
    assert_eq(code, 0, "okfw root exits 0")
    assert_true(out:find("okfw", 1, true) ~= nil, "okfw root prints bundle path")
end

-- Test 27: create --path places a concept in a subdir with slug filename + OKF frontmatter
log("")
log("Test 27: create --path + frontmatter")
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

-- Test 28: insert_link produces a vanilla markdown link
log("")
log("Test 28: insert_link (markdown)")
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

-- Test 29: follow_link navigates a relative markdown link
-- (use a scratch dir outside the bundle so these fixtures aren't scanned)
log("")
log("Test 29: follow_link")
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

-- Test 30: CLI validate --okf passes on a conformant bundle
log("")
log("Test 30: validate --okf")
if plenary_ok then
    local code, _ = run_okfw({ "validate", "--okf" })
    assert_eq(code, 0, "okfw validate --okf exits 0 on a conformant bundle")
end

-- Test 31: CLI index-md writes a root index.md with okf_version
log("")
log("Test 31: index-md")
if plenary_ok then
    local code, _ = run_okfw({ "index-md" })
    assert_eq(code, 0, "okfw index-md exits 0")
    local root_index = bundle .. "/index.md"
    if vim.fn.filereadable(root_index) == 1 then
        local fm = table.concat(vim.fn.readfile(root_index), "\n")
        assert_true(fm:find("okf_version", 1, true) ~= nil, "root index.md declares okf_version")
    end
end

-- Test 32: CLI search --kind filter
log("")
log("Test 32: search --kind")
if plenary_ok then
    local code, out = run_okfw({ "search", "--kind", "note", "--json" })
    assert_eq(code, 0, "okfw search --kind note exits 0")
    local ok2, _ = pcall(vim.json.decode, out)
    assert_true(ok2, "search --kind JSON is valid")
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
