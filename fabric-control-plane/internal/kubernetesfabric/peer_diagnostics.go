package kubernetesfabric

// Operation labels are fixed protocol names, never request arguments or errors.
func peerOperation(args []string) string {
	if len(args) >= 3 && args[0] == "lifecycle" && args[1] == "chaincode" {
		switch args[2] {
		case "install", "queryinstalled", "approveformyorg", "queryapproved", "checkcommitreadiness", "commit", "querycommitted":
			return "lifecycle-" + args[2]
		}
	}
	if len(args) >= 2 {
		if args[0] == "channel" {
			switch args[1] {
			case "join", "list", "fetch", "update":
				return "channel-" + args[1]
			}
		}
		if args[0] == "chaincode" {
			switch args[1] {
			case "query", "invoke":
				return "chaincode-" + args[1]
			}
		}
	}
	return "unclassified"
}
