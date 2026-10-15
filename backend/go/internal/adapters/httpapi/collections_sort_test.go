package httpapi_test

import (
	"net/http"
	"testing"
)

// The detail's view sort reuses the work listing's keys. Saved order is
// RJ3, RJ1, RJ5, RJ2, RJ4; the fixture makes every key tie or lack a value.
func TestCollectionsViewSort(t *testing.T) {
	server, repo := collectionServer(t)
	seedCollectionWorks(t, repo, "RJ00000001", "RJ00000002", "RJ00000003", "RJ00000004", "RJ00000005")
	for _, stmt := range []string{
		"UPDATE works SET release_date='2020-01-01' WHERE id IN ('RJ00000001','RJ00000004')",
		"UPDATE works SET release_date='2021-01-01' WHERE id='RJ00000002'",
		"UPDATE works SET collected_at='2026-01-01T00:00:00Z' WHERE id='RJ00000001'",
		"UPDATE works SET collected_at='2026-01-02T00:00:00Z' WHERE id='RJ00000002'",
		"UPDATE works SET collected_at='2026-01-03T00:00:00Z' WHERE id='RJ00000003'",
		"UPDATE works SET collected_at='2026-01-04T00:00:00Z' WHERE id IN ('RJ00000004','RJ00000005')",
		"INSERT INTO work_user_data(work_id,rating) VALUES('RJ00000001',5),('RJ00000004',5),('RJ00000002',9)",
	} {
		if _, err := repo.DB().Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	path := newCollectionPath(t, server, "Sorted")
	collectionRequest(t, server, http.MethodPost, path+"/works", map[string]any{"rjs": []string{
		"RJ00000003", "RJ00000001", "RJ00000005", "RJ00000002", "RJ00000004"}}, 200)

	for _, test := range []struct {
		query string
		want  []string
	}{
		{"", []string{"3", "1", "5", "2", "4"}},
		{"sort=", []string{"3", "1", "5", "2", "4"}},
		{"direction=desc", []string{"4", "2", "5", "1", "3"}},
		{"sort=rj", []string{"1", "2", "3", "4", "5"}},
		{"sort=rj&direction=desc", []string{"5", "4", "3", "2", "1"}},
		// Ties by saved position; unknown dates trail in both directions.
		{"sort=release_date", []string{"1", "4", "2", "3", "5"}},
		{"sort=release_date&direction=desc", []string{"2", "1", "4", "3", "5"}},
		{"sort=collected_at", []string{"1", "2", "3", "5", "4"}},
		{"sort=collected_at&direction=desc", []string{"5", "4", "3", "2", "1"}},
		// Unrated works trail in both directions.
		{"sort=rating", []string{"1", "4", "2", "3", "5"}},
		{"sort=rating&direction=desc", []string{"2", "1", "4", "3", "5"}},
	} {
		want := make([]string, len(test.want))
		for i, n := range test.want {
			want[i] = "RJ0000000" + n
		}
		assertCollectionWorks(t, server, path+"?"+test.query, want...)
	}

	// Paging walks one sorted order: the offset window is cut after sorting.
	assertCollectionWorks(t, server, path+"?sort=rating&direction=desc&limit=2&page=1", "RJ00000002", "RJ00000001")
	assertCollectionWorks(t, server, path+"?sort=rating&direction=desc&limit=2&page=2", "RJ00000004", "RJ00000003")
	page := collectionRequest(t, server, http.MethodGet, path+"?sort=rating&limit=2&page=3", nil, 200)
	if page["total"] != float64(5) {
		t.Fatalf("sorted total = %v", page)
	}
	assertCollectionWorks(t, server, path+"?sort=rating&limit=2&page=3", "RJ00000005")

	// The view sort never rewrites the saved order.
	assertCollectionWorks(t, server, path, "RJ00000003", "RJ00000001", "RJ00000005", "RJ00000002", "RJ00000004")

	for _, query := range []string{"sort=title", "sort=RJ", "sort=position"} {
		result := collectionRequest(t, server, http.MethodGet, path+"?"+query, nil, 400)
		if result["code"] != "INVALID_ARGUMENT" {
			t.Fatalf("%s refusal = %v", query, result)
		}
	}
	result := collectionRequest(t, server, http.MethodGet, "/api/v1/collections?sort=rj", nil, 400)
	if result["code"] != "INVALID_ARGUMENT" {
		t.Fatalf("list sort refusal = %v", result)
	}
}
