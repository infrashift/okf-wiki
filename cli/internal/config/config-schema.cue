#Config: {
	root_path:  string | *"~/okfw"
	index_path: string | *".okfw_index"
	graph_path: string | *".okfw_graph.db" // Persistent graph database
	todos_path: string | *".okfw_todos"    // Generated todo lists
	editor:     string | *"nvim"
	folders: {
		untethered: string | *"untethered"
		tethered:   string | *"tethered"
		tmp:        string | *"tmp"
	}
}
