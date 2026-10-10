// Spatial public values retain their existing strict scalar and canonical-pose checks.
package value

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
)

// PoseJSON is the sole public pose representation, shared by record projection
// and the existing mount command commitment. It does not validate a fact.
func PoseJSON(pose contract.Pose) map[string]any {
	r := pose.Rotation.Ratios()
	return map[string]any{"translation": []any{pose.Translation.X, pose.Translation.Y, pose.Translation.Z}, "rotation": map[string]any{"pivot": float64(pose.Rotation.Pivot()), "ratios": []any{r[0], r[1], r[2]}}}
}

func Vec3(value any) (contract.Vec3, error) {
	array, ok := value.([]any)
	if !ok || len(array) != 3 {
		return contract.Vec3{}, values.PublicInvalid
	}
	var n [3]float64
	for i := range array {
		var ok bool
		n[i], ok = array[i].(float64)
		if !ok || !geometry.Finite(n[i]) {
			return contract.Vec3{}, values.PublicInvalid
		}
	}
	return contract.Vec3{X: n[0], Y: n[1], Z: n[2]}, nil
}

func ParsePose(raw any) (contract.Pose, error) {
	pose, err := Object(raw, "translation", "rotation")
	if err != nil {
		return contract.Pose{}, err
	}
	t, err := Vec3(pose["translation"])
	if err != nil {
		return contract.Pose{}, err
	}
	r, err := Object(pose["rotation"], "pivot", "ratios")
	if err != nil {
		return contract.Pose{}, err
	}
	pivot, ok := r["pivot"].(float64)
	if !ok || pivot < 1 || pivot > 4 || float64(uint8(pivot)) != pivot {
		return contract.Pose{}, values.PublicInvalid
	}
	ratios, err := Vec3(r["ratios"])
	if err != nil {
		return contract.Pose{}, err
	}
	result := contract.Pose{Translation: t, Rotation: geometry.RotationFromComponents(contract.RotationPivot(pivot), [3]float64{ratios.X, ratios.Y, ratios.Z})}
	if !geometry.CanonicalPose(result) {
		return contract.Pose{}, contract.ErrInvalid
	}
	return result, nil
}
