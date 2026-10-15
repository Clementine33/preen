package httpapi_test

import (
	"net/http"
	"slices"
	"testing"
)

// The work detail names the user collections holding the work in list order;
// Favorites is stated by the favorite flag instead.
func TestWorkDetailCollectionIDs(t *testing.T) {
	server, repo := collectionServer(t)
	seedCollectionWorks(t, repo, "RJ00000001")
	ids := func() []string {
		detail := collectionRequest(t, server, http.MethodGet, "/api/v1/works/RJ00000001", nil, 200)
		raw, ok := detail["collection_ids"].([]any)
		if !ok {
			t.Fatalf("collection_ids = %v, want an array", detail["collection_ids"])
		}
		out := []string{}
		for _, v := range raw {
			out = append(out, collectionValue[string](t, v))
		}
		return out
	}
	if got := ids(); len(got) != 0 {
		t.Fatalf("no collections: %v", got)
	}
	var created []string
	for i, name := range []string{"first", "second", "third"} {
		path := newCollectionPath(t, server, name)
		id := path[len("/api/v1/collections/"):]
		stamp := "2026-10-14T00:00:0" + string(rune('1'+i)) + "Z"
		if _, err := repo.DB().Exec("UPDATE collections SET created_at=? WHERE id=?", stamp, id); err != nil {
			t.Fatal(err)
		}
		created = append(created, id)
	}
	add := func(path string) {
		collectionRequest(t, server, http.MethodPost, path+"/works", map[string]any{"rjs": []string{"RJ00000001"}}, 200)
	}
	add("/api/v1/collections/" + created[2])
	add("/api/v1/collections/favorites")
	add("/api/v1/collections/" + created[0])
	if got := ids(); !slices.Equal(got, []string{created[0], created[2]}) {
		t.Fatalf("ids = %v, want %v", got, []string{created[0], created[2]})
	}
	collectionRequest(t, server, http.MethodDelete, "/api/v1/collections/"+created[0]+"/works/RJ00000001", nil, 204)
	if got := ids(); !slices.Equal(got, []string{created[2]}) {
		t.Fatalf("after remove: %v", got)
	}
	collectionRequest(t, server, http.MethodDelete, "/api/v1/collections/"+created[2], nil, 204)
	if got := ids(); len(got) != 0 {
		t.Fatalf("after delete: %v", got)
	}
}
