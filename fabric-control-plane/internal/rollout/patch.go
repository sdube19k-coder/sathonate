package rollout

import (
	"encoding/json"
	"errors"
	"strconv"
)

// ImagePatch limits the write to the selected container image. The caller must
// address the plan's exact Deployment through its already bound Kubernetes client.
func ImagePatch(plan Plan) ([]byte, error) {
	c := plan.Component
	if c.UID == "" || c.ResourceVersion == "" || c.ContainerIndex < 0 ||
		!componentID.MatchString(c.Container) || !imageDigest.MatchString(c.Image) ||
		!imageDigest.MatchString(plan.TargetImage) || plan.TargetImage == c.Image {
		return nil, errors.New("invalid image patch preconditions")
	}
	type operation struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}
	container := "/spec/template/spec/containers/" + strconv.Itoa(c.ContainerIndex)
	return json.Marshal([]operation{
		{"test", "/metadata/uid", c.UID},
		{"test", "/metadata/resourceVersion", c.ResourceVersion},
		{"test", container + "/name", c.Container},
		{"test", container + "/image", c.Image},
		{"replace", container + "/image", plan.TargetImage},
	})
}
