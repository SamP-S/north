package tasks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/models"
)

// SortKey names a task ordering.
type SortKey string

const (
	SortID       SortKey = "id"
	SortUpdated  SortKey = "updated"
	SortTitle    SortKey = "title"
	SortAssignee SortKey = "assignee"
)

// SortKeys lists every ordering in display order.
var SortKeys = []SortKey{SortID, SortUpdated, SortTitle, SortAssignee}

// ParseSortKey coerces a string to a SortKey, raising Invalid on unknown values.
func ParseSortKey(value string) (SortKey, error) {
	k := SortKey(value)
	for _, s := range SortKeys {
		if k == s {
			return k, nil
		}
	}
	allowed := make([]string, len(SortKeys))
	for i, s := range SortKeys {
		allowed[i] = string(s)
	}
	return "", errors.Invalid(fmt.Sprintf("unknown sort key %q (expected one of: %s)",
		value, strings.Join(allowed, ", ")))
}

// Sort orders tasks by key, descending when desc is set. Ties fall back to
// ascending numeric id. Unassigned tasks sort last under the assignee key
// regardless of direction.
func Sort(ts []*models.Task, key SortKey, desc bool) {
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		if key == SortAssignee {
			ae, be := a.Assignee == "", b.Assignee == ""
			if ae != be {
				return be // exactly one unassigned → the assigned one first
			}
		}
		if c := compare(a, b, key); c != 0 {
			if desc {
				return c > 0
			}
			return c < 0
		}
		return idNum(a.ID) < idNum(b.ID)
	})
}

// compare returns -1/0/1 for a vs b under key (ascending sense).
func compare(a, b *models.Task, key SortKey) int {
	switch key {
	case SortUpdated:
		au, bu := int64(0), int64(0)
		if a.UpdatedAt != nil {
			au = a.UpdatedAt.UnixNano()
		}
		if b.UpdatedAt != nil {
			bu = b.UpdatedAt.UnixNano()
		}
		return cmp(au, bu)
	case SortTitle:
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	case SortAssignee:
		return strings.Compare(strings.ToLower(a.Assignee), strings.ToLower(b.Assignee))
	default: // SortID
		return cmp(int64(idNum(a.ID)), int64(idNum(b.ID)))
	}
}

func cmp(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
