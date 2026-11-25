package routes

import (
	"net/http"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/invite"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func (a *API) createInvite(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	issued, err := a.svc.Invites.Create(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	out := toInvite(issued.Invite)
	out.Token, out.URL = issued.Token, issued.URL
	return details(w, http.StatusCreated, out)
}

func (a *API) listInvites(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	invites, err := a.svc.Invites.List(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	out := make([]platform.Invite, 0, len(invites))
	for _, i := range invites {
		out = append(out, toInvite(i))
	}
	return paged(w, r, out)
}

func (a *API) deleteInvite(w http.ResponseWriter, r *http.Request) error {
	groupID, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	id, err := pathID(r, "inviteId", "Invite")
	if err != nil {
		return err
	}
	if err := a.svc.Invites.Revoke(r.Context(), me(r), groupID, id); err != nil {
		return err
	}
	return noContent(w)
}

func (a *API) previewInvite(w http.ResponseWriter, r *http.Request) error {
	p, err := a.svc.Invites.Preview(r.Context(), r.PathValue("token"))
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	return details(w, http.StatusOK, platform.InvitePreview{GroupName: p.Group.Name, Emoji: p.Group.Emoji, Color: p.Group.Color,
		MemberCount: len(p.Group.Members), InvitedBy: p.Inviter.Name})
}

func (a *API) acceptInvite(w http.ResponseWriter, r *http.Request) error {
	v, err := a.svc.Invites.Accept(r.Context(), me(r), r.PathValue("token"))
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toGroup(v))
}

func toInvite(i *invite.Invite) platform.Invite {
	return platform.Invite{ID: i.ID.String(), CreatedBy: i.CreatedBy.String(), ExpiresAt: stamp(i.ExpiresAt), CreatedAt: stamp(i.CreatedAt)}
}
