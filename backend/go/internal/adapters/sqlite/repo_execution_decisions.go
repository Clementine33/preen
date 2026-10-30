package sqlite

import "encoding/json"

// GetPlanComponentOutcomes loads only the frozen outcomes this execution page
// carries. json_each keeps a full snapshot within SQLite's parameter limit.
func (r *Repository) GetPlanComponentOutcomes(planID string, indexes []int) (map[int]json.RawMessage, error) {
	outcomes := make(map[int]json.RawMessage, len(indexes))
	if len(indexes) == 0 {
		return outcomes, nil
	}
	raw, err := json.Marshal(indexes)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`
		SELECT component_index, outcome_json FROM conversion_components
		WHERE plan_id = ? AND plan_id IN (`+visiblePlanIDs+`)
		  AND component_index IN (SELECT value FROM json_each(?))
	`, planID, string(raw))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var index int
		var outcome string
		if err := rows.Scan(&index, &outcome); err != nil {
			return nil, err
		}
		outcomes[index] = json.RawMessage(outcome)
	}
	return outcomes, rows.Err()
}
