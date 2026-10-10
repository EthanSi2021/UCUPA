package namespace

import (
	"encoding/json"
)

// byteString preserves opaque caller IDs exactly across private JSON storage;
// JSON text strings would silently replace invalid UTF-8.
type byteString string

func (s byteString) MarshalJSON() ([]byte, error) { return json.Marshal([]byte(s)) }
func (s *byteString) UnmarshalJSON(raw []byte) error {
	var b []byte
	if err := json.Unmarshal(raw, &b); err != nil {
		return err
	}
	*s = byteString(b)
	return nil
}

type actorData struct {
	Realm     byteString
	Principal byteString
}

func (a actorData) Valid() bool { return a.Realm != "" && a.Principal != "" }
