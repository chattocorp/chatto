package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// applyBootstrapEnv owns the indexed environment format for development-only
// bootstrap data that cannot be represented by fixed struct tags.
func applyBootstrapEnv(cfg *ChattoConfig) error {
	users, usersSet, err := bootstrapUsersFromEnv()
	if err != nil {
		return err
	}
	if usersSet {
		cfg.Bootstrap.Users = users
	}

	name, nameSet := os.LookupEnv("CHATTO_BOOTSTRAP_SERVER_NAME")
	rooms, roomsSet := os.LookupEnv("CHATTO_BOOTSTRAP_SERVER_ROOMS")
	if nameSet || roomsSet {
		cfg.Bootstrap.Server = &BootstrapServer{Name: name}
		if roomsSet {
			cfg.Bootstrap.Server.Rooms = splitCommaSeparatedEnv(rooms)
		}
	}
	return nil
}

func bootstrapUsersFromEnv() ([]BootstrapUser, bool, error) {
	const prefix = "CHATTO_BOOTSTRAP_USERS_"
	usersByIndex := make(map[int]*BootstrapUser)

	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, prefix) {
			continue
		}

		rest := strings.TrimPrefix(name, prefix)
		indexPart, field, ok := strings.Cut(rest, "_")
		if !ok {
			return nil, false, fmt.Errorf("%s must use CHATTO_BOOTSTRAP_USERS_<index>_<field>", name)
		}
		index, err := strconv.Atoi(indexPart)
		if err != nil || index < 0 {
			return nil, false, fmt.Errorf("%s uses invalid bootstrap user index %q", name, indexPart)
		}

		user := usersByIndex[index]
		if user == nil {
			user = &BootstrapUser{}
			usersByIndex[index] = user
		}
		switch field {
		case "LOGIN":
			user.Login = value
		case "DISPLAY_NAME":
			user.DisplayName = value
		case "EMAIL":
			user.Email = value
		case "PASSWORD":
			user.Password = value
		case "SERVER_ROLE":
			user.ServerRole = value
		default:
			return nil, false, fmt.Errorf("%s uses unknown bootstrap user field %q", name, field)
		}
	}

	if len(usersByIndex) == 0 {
		return nil, false, nil
	}
	indices := make([]int, 0, len(usersByIndex))
	for index := range usersByIndex {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	users := make([]BootstrapUser, 0, len(indices))
	for expected, index := range indices {
		if index != expected {
			return nil, false, fmt.Errorf("CHATTO_BOOTSTRAP_USERS_* indexes must be contiguous starting at 0; missing index %d", expected)
		}
		users = append(users, *usersByIndex[index])
	}
	return users, true, nil
}
