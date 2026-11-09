package service

import (
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/hostlabel"
)

// ResolveWebHostName finds the space a web host's name part routes to for a
// user: a space or alt name of theirs or shared with them, or a member of
// their pool, by name, or by its shortened form when the whole host would
// be longer than DNS allows (see hostlabel). Nil when none.
func ResolveWebHostName(userId, part string) *model.Space {
	db := database.GetInstance()
	if space, err := db.GetSpaceByName(userId, part); err == nil && space != nil {
		return space
	}
	if space := GetPoolService().PickMemberForRouting(part, userId); space != nil {
		return space
	}
	if !hostlabel.Shortened(part) {
		return nil
	}

	if spaces, err := db.GetSpacesForUser(userId); err == nil {
		for _, s := range spaces {
			if s.IsDeleted || (s.UserId != userId && !s.IsSharedWith(userId)) {
				continue
			}
			if hostlabel.Matches(part, s.Name) {
				return s
			}
			for _, alt := range s.AltNames {
				if hostlabel.Matches(part, alt.Name) {
					return s
				}
			}
		}
	}
	if pools, err := db.GetPoolDefinitions(); err == nil {
		for _, p := range pools {
			if p.IsDeleted || p.CreatedUserId != userId || !hostlabel.Matches(part, p.Name) {
				continue
			}
			if space := GetPoolService().PickMemberForRouting(p.Name, userId); space != nil {
				return space
			}
		}
	}
	return nil
}
