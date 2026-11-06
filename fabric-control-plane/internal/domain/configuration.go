package domain

const DynamicLocalProfile = "smp-dynamic-local-v1"

type Configuration struct {
	Placements  map[string]Placement    `json:"placements"`
	Nodes       map[string]NodeSettings `json:"nodes"`
	Authorities []Authority             `json:"authorities"`
	Anchors     map[string][]string     `json:"anchors"`
	Chaincodes  []Chaincode             `json:"chaincodes"`
}
type Placement struct {
	NodeSelector map[string]string `json:"nodeSelector"`
}
type Resources struct {
	CPURequestMilli  int `json:"cpuRequestMilli"`
	CPULimitMilli    int `json:"cpuLimitMilli"`
	MemoryRequestMiB int `json:"memoryRequestMiB"`
	MemoryLimitMiB   int `json:"memoryLimitMiB"`
}
type Storage struct {
	Class       string `json:"class"`
	CapacityGiB int    `json:"capacityGiB"`
}
type NodeSettings struct {
	Resources Resources `json:"resources"`
	Storage   Storage   `json:"storage"`
}
type Authority struct {
	ID              string    `json:"id"`
	OrganizationRef string    `json:"organizationRef"`
	Roles           []string  `json:"roles"`
	PlacementRef    string    `json:"placementRef"`
	Resources       Resources `json:"resources"`
	Storage         Storage   `json:"storage"`
}
type Endorsement struct {
	MSPIDs  []string `json:"mspIds"`
	Minimum int      `json:"minimum"`
}
type Collection struct {
	Name              string      `json:"name"`
	Members           Endorsement `json:"members"`
	RequiredPeerCount int         `json:"requiredPeerCount"`
	MaximumPeerCount  int         `json:"maximumPeerCount"`
	BlockToLive       uint64      `json:"blockToLive"`
}
type Chaincode struct {
	Name          string       `json:"name"`
	ChannelRef    string       `json:"channelRef"`
	Version       string       `json:"version"`
	Sequence      int          `json:"sequence"`
	PackageSHA256 string       `json:"packageSHA256"`
	RuntimeImage  string       `json:"runtimeImage"`
	Endorsement   Endorsement  `json:"endorsement"`
	Collections   []Collection `json:"collections,omitempty"`
}
