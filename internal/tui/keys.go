package tui

// keyHelpRow is one line of the ? help overlay.
type keyHelpRow struct {
	keys   string
	panel  string
	action string
}

// keyHelpRows lists every binding; keep it in sync with updateKey and the README.
var keyHelpRows = []keyHelpRow{
	{"j / k", "any", "move down / up"},
	{"g / G, < / >", "any", "jump to top / bottom"},
	{"ctrl+d / ctrl+u, . / ,", "any", "move half a page down / up"},
	{"h / l, tab, 0 1 2 3", "any", "switch panel"},
	{"r", "any", "refresh the host's listening ports"},
	{"A", "any", "add a remote forward"},
	{"?", "any", "show or hide this help"},
	{"q", "any", "stop all forwards and quit"},
	{"space, s", "Live", "start or stop the forward"},
	{"enter", "Live", "show the forward in its host's Forwards"},
	{"e", "Live", "edit the forward"},
	{"d", "Live", "delete the forward"},
	{"o", "Live", "open the laptop end in the browser"},
	{"a", "Hosts", "add a host"},
	{"p", "Hosts", "pin or unpin the host"},
	{"d", "Hosts", "remove an added host that has no forwards"},
	{"enter", "Hosts", "open the host's forwards"},
	{"a", "Forwards", "add a local forward"},
	{"space, s", "Forwards", "start or stop the forward"},
	{"e", "Forwards", "edit the forward"},
	{"d", "Forwards", "delete the forward"},
	{"o", "Forwards", "open the laptop end in the browser"},
	{"a", "Listening", "add a local forward"},
	{"space, enter, f", "Listening", "forward this port, or start or stop its forward"},
	{"t", "Listening", "show your ports or all ports"},
}
