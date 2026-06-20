package model

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

type UserKind string

const (
	UserKindHuman UserKind = "HUMAN"
	UserKindBot   UserKind = "BOT"
)

var AllUserKind = []UserKind{
	UserKindHuman,
	UserKindBot,
}

func (e UserKind) IsValid() bool {
	switch e {
	case UserKindHuman, UserKindBot:
		return true
	}
	return false
}

func (e UserKind) String() string {
	return string(e)
}

func (e *UserKind) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}
	*e = UserKind(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid UserKind", str)
	}
	return nil
}

func (e UserKind) MarshalGQL(w io.Writer) {
	_, _ = fmt.Fprint(w, strconv.Quote(string(e)))
}

func (e *UserKind) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*e = UserKind(s)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid UserKind", s)
	}
	return nil
}

func (e UserKind) MarshalJSON() ([]byte, error) {
	if !e.IsValid() {
		return nil, fmt.Errorf("%s is not a valid UserKind", e)
	}
	return json.Marshal(e.String())
}
