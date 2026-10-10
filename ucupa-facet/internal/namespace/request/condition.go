package request

import "ucupa/shared/values"

// CommandCondition decodes the existing put/delete expected condition. The
// put form is exactly one of {absent:true} or {revision:<record token>}; delete
// accepts only the record token form. It does not compare the token to state.
func CommandCondition(r map[string]any, kind string) (bool, values.PublicToken, error) {
	value := r["expected"]
	if kind == "put" {
		condition, err := Object(value, "absent", "revision")
		if err != nil || len(condition) != 1 {
			return false, values.PublicToken{}, values.PublicInvalid
		}
		if absent, ok := condition["absent"]; ok {
			if absent != true {
				return false, values.PublicToken{}, values.PublicInvalid
			}
			return true, values.PublicToken{}, nil
		}
		value = condition["revision"]
	}
	token, err := Token(value, "record-revision")
	return false, token, err
}
