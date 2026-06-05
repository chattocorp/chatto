package model

import (
	"fmt"
	"io"
	"strconv"
)

// TimeFormat represents the user's preferred time display format.
type TimeFormat string

const (
	TimeFormatAuto           TimeFormat = "AUTO"
	TimeFormatTwelveHour     TimeFormat = "TWELVE_HOUR"
	TimeFormatTwentyFourHour TimeFormat = "TWENTY_FOUR_HOUR"
)

var AllTimeFormat = []TimeFormat{
	TimeFormatAuto,
	TimeFormatTwelveHour,
	TimeFormatTwentyFourHour,
}

func (e TimeFormat) IsValid() bool {
	switch e {
	case TimeFormatAuto, TimeFormatTwelveHour, TimeFormatTwentyFourHour:
		return true
	}
	return false
}

func (e TimeFormat) String() string {
	return string(e)
}

func (e *TimeFormat) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = TimeFormat(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid TimeFormat", str)
	}
	return nil
}

func (e TimeFormat) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

// UserSettings represents a user's display preferences.
type UserSettings struct {
	Timezone   *string    `json:"timezone"`
	TimeFormat TimeFormat `json:"timeFormat"`
}
