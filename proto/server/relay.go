package main

import (
	"context"
	"slices"
	"time"

	"github.com/fiatjaf/eventstore"
	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/khatru/policies"
	"github.com/fiatjaf/relay29"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip29"
)

// The relay hosts two NIP-29 groups. Both are closed (membership comes only
// from registration) and private (reading requires NIP-42 auth as a member).
const (
	CommunityGroup     = "alhovuori"
	AnnouncementsGroup = "announcements"
)

var adminRole = &nip29.Role{Name: "admin", Description: "community administrator"}

type Relay struct {
	*khatru.Relay
	State  *relay29.State
	Pubkey string
}

// NewRelay builds the community relay. It mirrors khatru29.Init, with three
// changes: members may publish and read kind-0 profile metadata (which has no
// group tag), groups can only be created by the relay itself, and only admins
// may post in the announcements group.
func NewRelay(domain, secretKey string, db eventstore.Store) *Relay {
	pubkey, _ := nostr.GetPublicKey(secretKey)
	state := relay29.New(relay29.Options{
		Domain:                  domain,
		DB:                      db,
		SecretKey:               secretKey,
		DefaultRoles:            []*nip29.Role{adminRole},
		GroupCreatorDefaultRole: adminRole,
	})
	kr := khatru.NewRelay()
	kr.Info.PubKey = pubkey
	kr.Info.Name = "Alhovuori community relay"
	kr.Info.Description = "Private relay of the Alhovuori community. Membership comes from registration."
	kr.Info.SupportedNIPs = append(kr.Info.SupportedNIPs, 29)

	state.Relay = kr
	state.GetAuthed = khatru.GetAuthed
	state.AllowAction = func(ctx context.Context, group nip29.Group, role *nip29.Role, action relay29.Action) bool {
		return role == adminRole
	}

	r := &Relay{Relay: kr, State: state, Pubkey: pubkey}

	kr.StoreEvent = append(kr.StoreEvent, db.SaveEvent)
	kr.DeleteEvent = append(kr.DeleteEvent, db.DeleteEvent)
	kr.QueryEvents = append(kr.QueryEvents,
		state.NormalEventQuery,
		state.MetadataQueryHandler,
		state.AdminsQueryHandler,
		state.MembersQueryHandler,
		state.RolesQueryHandler,
		r.profileQuery,
	)
	kr.RejectFilter = append(kr.RejectFilter, r.rejectFilter)
	kr.RejectEvent = append(kr.RejectEvent,
		policies.RestrictToSpecifiedKinds(true,
			uint16(nostr.KindProfileMetadata),
			uint16(nostr.KindSimpleGroupChatMessage),
			9000, 9001, 9002, 9005, 9006, 9007, 9008,
		),
		clientEventsOnly(policies.PreventTimestampsInTheFuture(30*time.Second)),
		policies.PreventLargeTags(64),
		r.requireGroupTag,
		state.RequireModerationEventsToBeRecent,
		r.restrictWrites,
		state.RestrictInvalidModerationActions,
		state.PreventWritingOfEventsJustDeleted,
		state.CheckPreviousTag,
	)
	kr.OnEventSaved = append(kr.OnEventSaved,
		state.ApplyModerationAction,
		r.addToPreviousChecking,
	)
	kr.OnConnect = append(kr.OnConnect, khatru.RequestAuth)
	return r
}

// clientEventsOnly skips a policy for events the relay generates itself:
// relay29 spaces those out by bumping created_at one second per event, so a
// burst of registrations would otherwise trip the future-timestamp check.
func clientEventsOnly(policy func(context.Context, *nostr.Event) (bool, string)) func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, evt *nostr.Event) (bool, string) {
		if relay29.IsInternalCall(ctx) {
			return false, ""
		}
		return policy(ctx, evt)
	}
}

func isProfileFilter(f nostr.Filter) bool {
	return len(f.Kinds) > 0 && !slices.ContainsFunc(f.Kinds, func(k int) bool { return k != nostr.KindProfileMetadata })
}

// IsMember reports whether pubkey belongs to the community group.
func (r *Relay) IsMember(pubkey string) bool {
	group, _ := r.State.Groups.Load(CommunityGroup)
	if group == nil {
		return false
	}
	_, ok := group.Members[pubkey]
	return ok
}

func (r *Relay) isAdmin(group *relay29.Group, pubkey string) bool {
	return slices.Contains(group.Members[pubkey], adminRole)
}

func (r *Relay) rejectFilter(ctx context.Context, f nostr.Filter) (bool, string) {
	if !isProfileFilter(f) {
		return r.State.RequireKindAndSingleGroupIDOrSpecificEventReference(ctx, f)
	}
	authed := khatru.GetAuthed(ctx)
	if authed == "" {
		return true, "auth-required: profiles are visible to members only"
	}
	if !r.IsMember(authed) {
		return true, "restricted: not a member"
	}
	return false, ""
}

func (r *Relay) profileQuery(ctx context.Context, f nostr.Filter) (chan *nostr.Event, error) {
	if !isProfileFilter(f) {
		ch := make(chan *nostr.Event)
		close(ch)
		return ch, nil
	}
	return r.State.DB.QueryEvents(ctx, f)
}

func (r *Relay) requireGroupTag(ctx context.Context, evt *nostr.Event) (bool, string) {
	if evt.Kind == nostr.KindProfileMetadata {
		return false, ""
	}
	return r.State.RequireHTagForExistingGroup(ctx, evt)
}

func (r *Relay) restrictWrites(ctx context.Context, evt *nostr.Event) (bool, string) {
	if evt.PubKey == r.Pubkey {
		return false, ""
	}
	switch evt.Kind {
	case nostr.KindProfileMetadata:
		if !r.IsMember(evt.PubKey) {
			return true, "restricted: only members can publish profiles here"
		}
		return false, ""
	case nostr.KindSimpleGroupCreateGroup:
		return true, "restricted: groups are managed by the community"
	case nostr.KindSimpleGroupChatMessage:
		group := r.State.GetGroupFromEvent(evt)
		if group.Address.ID == AnnouncementsGroup && !r.isAdmin(group, evt.PubKey) {
			return true, "restricted: only administrators post announcements"
		}
	}
	return r.State.RestrictWritesBasedOnGroupRules(ctx, evt)
}

func (r *Relay) addToPreviousChecking(ctx context.Context, evt *nostr.Event) {
	if evt.Kind != nostr.KindProfileMetadata {
		r.State.AddToPreviousChecking(ctx, evt)
	}
}

// Bootstrap creates the community groups on first start and makes sure every
// configured administrator holds the admin role in both.
func (r *Relay) Bootstrap(ctx context.Context, admins []string) error {
	groups := []struct{ id, name, about string }{
		{CommunityGroup, "Alhovuori", "The Alhovuori community message board."},
		{AnnouncementsGroup, "Alhovuori announcements", "Announcements from the community administrators."},
	}
	yes := true
	for _, g := range groups {
		if group, _ := r.State.Groups.Load(g.id); group == nil {
			name, about := g.name, g.about
			err := r.State.CreateGroup(ctx, g.id, r.Pubkey, relay29.EditMetadata{
				NameValue: &name, AboutValue: &about, ClosedValue: &yes, PrivateValue: &yes,
			})
			if err != nil {
				return err
			}
		}
		group, _ := r.State.Groups.Load(g.id)
		for _, admin := range admins {
			if !r.isAdmin(group, admin) {
				if err := r.State.PutUser(ctx, g.id, admin, adminRole.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// AddMember puts a newly registered member into both groups.
func (r *Relay) AddMember(ctx context.Context, pubkey string) error {
	for _, id := range []string{CommunityGroup, AnnouncementsGroup} {
		if err := r.State.PutUser(ctx, id, pubkey); err != nil {
			return err
		}
	}
	return nil
}

// RemoveMember takes a member out of both groups, e.g. when an old identity is
// retired as part of identity migration.
func (r *Relay) RemoveMember(ctx context.Context, pubkey string) error {
	for _, id := range []string{CommunityGroup, AnnouncementsGroup} {
		if err := r.State.RemoveUserFromGroup(ctx, id, pubkey); err != nil {
			return err
		}
	}
	return nil
}
