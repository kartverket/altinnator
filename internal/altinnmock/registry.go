package altinnmock

import (
	"encoding/json"
	"sync"
	"time"
)

type version struct {
	versionID int64
	resource  map[string]any
	updatedAt string
}

type stored struct {
	createdAt string
	newest    version
}

// Altinn's Resource Registry in memory. Altinn keeps every version of a resource, but Altinnator only reads the newest,
// so the mock keeps only that
type registry struct {
	mu        sync.Mutex
	resources map[string]*stored
	// Like Altinn's, a version id counts up across all resources
	lastVersionID int64
}

func newRegistry() *registry {
	return &registry{resources: map[string]*stored{}}
}

// Like Altinn's, the id must match exactly, case included
func (r *registry) get(id string) (map[string]any, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resource, ok := r.resources[id]
	if !ok {
		return nil, false
	}
	return withMetadata(resource), true
}

func (r *registry) create(resource map[string]any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := resource["identifier"].(string)
	if _, taken := r.resources[id]; taken {
		return false
	}
	now := now()
	r.lastVersionID++
	r.resources[id] = &stored{createdAt: now, newest: version{r.lastVersionID, resource, now}}
	return true
}

func (r *registry) put(resource map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := resource["identifier"].(string)
	now := now()
	r.lastVersionID++
	if existing, ok := r.resources[id]; ok {
		existing.newest = version{r.lastVersionID, resource, now}
		return
	}
	r.resources[id] = &stored{createdAt: now, newest: version{r.lastVersionID, resource, now}}
}

func withMetadata(resource *stored) map[string]any {
	// A stored resource was copied once already, so copying it again cannot fail
	result, _ := deepCopy(resource.newest.resource)
	result["versionId"] = resource.newest.versionID
	result["createdAt"] = resource.createdAt
	result["updatedAt"] = resource.newest.updatedAt
	return result
}

func deepCopy(value map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, json.Unmarshal(raw, &result)
}

// Like Altinn's: microseconds, and an offset of +00:00
func now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
}
