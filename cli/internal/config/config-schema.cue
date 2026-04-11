#Config: {
	root_path:  string | *"~/zk_vault"
	index_path: string | *".zk_index"
	graph_path: string | *".zk_graph.db" // Persistent graph database
	todos_path: string | *".zk_todos"    // Generated todo lists
	editor:     string | *"nvim"
	folders: {
		untethered: string | *"untethered"
		tethered:   string | *"tethered"
		tmp:        string | *"tmp"
	}
}
