package rollout

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImagePatchMockKubernetesConflict(t *testing.T) {
	previous := "registry.test/fabric@sha256:" + strings.Repeat("a", 64)
	next := "registry.test/fabric@sha256:" + strings.Repeat("b", 64)
	patch, err := ImagePatch(Plan{Component: Component{UID: "approved-uid", ResourceVersion: "42", Container: "peer", Image: previous}, TargetImage: next})
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		uid, version, container, image string
		status                         int
	}{
		{"approved-uid", "42", "peer", previous, 200},
		{"replacement-uid", "42", "peer", previous, 409},
		{"approved-uid", "43", "peer", previous, 409},
		{"approved-uid", "42", "sidecar", previous, 409},
		{"approved-uid", "42", "peer", next, 409},
	} {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PATCH" || r.URL.Path != "/apis/apps/v1/namespaces/fixture/deployments/peer" ||
				r.URL.Query().Get("dryRun") != "All" || r.Header.Get("Content-Type") != "application/json-patch+json" {
				t.Fatal("mock request escaped dry-run contract")
			}
			var ops []struct{ Op, Path, Value string }
			if json.NewDecoder(r.Body).Decode(&ops) != nil || len(ops) != 5 {
				t.Fatal("invalid patch")
			}
			container := "/spec/template/spec/containers/0"
			state := map[string]string{
				"/metadata/uid": scenario.uid, "/metadata/resourceVersion": scenario.version,
				container + "/name": scenario.container, container + "/image": scenario.image,
			}
			for _, op := range ops[:4] {
				if op.Op != "test" || state[op.Path] != op.Value {
					w.WriteHeader(http.StatusConflict)
					return
				}
				delete(state, op.Path)
			}
			if len(state) != 0 || ops[4].Op != "replace" || ops[4].Path != container+"/image" || ops[4].Value != next {
				t.Fatal("patch changed a non-image field or skipped a precondition")
			}
			w.WriteHeader(http.StatusOK)
		})
		request := httptest.NewRequest("PATCH", "/apis/apps/v1/namespaces/fixture/deployments/peer?dryRun=All", strings.NewReader(string(patch)))
		request.Header.Set("Content-Type", "application/json-patch+json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != scenario.status {
			t.Fatalf("mock response %d, want %d", response.Code, scenario.status)
		}
	}
}
