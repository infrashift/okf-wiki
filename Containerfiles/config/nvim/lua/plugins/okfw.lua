return {
  {
    dir = "/home/user/okfw-plugin",
    name = "okfw.nvim",
    lazy = false,
    dependencies = {
      "nvim-lua/plenary.nvim",
      "folke/snacks.nvim",
    },
    config = function()
      require("okfw").setup({ bin = "/home/user/.local/bin/okfw" })
    end,
  },
}
