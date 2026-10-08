package intern_test

import (
	"fmt"

	"hmans.de/chatto/pkg/events/intern"
)

func ExampleTable() {
	var ids intern.Table[testKind]
	owner := ids.Intern("account-42")
	counts := map[intern.ID[testKind]]int{owner: 3}
	if handle, ok := ids.Lookup("account-42"); ok {
		fmt.Println(ids.Resolve(handle), counts[handle])
	}
	// Output: account-42 3
}
