package api

import (
	"net/http"
	"strconv"

	"github.com/atb00ker/splitspends/backend/internal/core/domains/group"
	"github.com/atb00ker/splitspends/backend/internal/platform"
)

func (a *API) listFriends(w http.ResponseWriter, r *http.Request) error {
	friends, err := a.svc.Friends.List(r.Context(), me(r))
	if err != nil {
		return err
	}
	out := make([]platform.Friend, 0, len(friends))
	for _, f := range friends {
		out = append(out, toFriend(f))
	}
	return list(w, out, len(out))
}

func (a *API) addFriend(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.Invitee](r)
	if err != nil {
		return err
	}
	f, err := a.svc.Friends.Add(r.Context(), me(r), in.Email, in.Name)
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, toFriend(*f))
}

func (a *API) getFriend(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "userId", "Friend")
	if err != nil {
		return err
	}
	d, err := a.svc.Friends.Detail(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	out := platform.FriendDetail{Friend: toFriend(d.Summary), Groups: []platform.FriendGroup{}, NonGroup: toBalances(d.NonGroup)}
	for _, g := range d.Groups {
		out.Groups = append(out.Groups, platform.FriendGroup{GroupID: g.GroupID.String(), GroupName: g.GroupName, Balances: toBalances(g.Balances)})
	}
	return details(w, http.StatusOK, out)
}

func (a *API) inviteFriend(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "userId", "Friend")
	if err != nil {
		return err
	}
	in, err := decode[platform.FriendInvite](r)
	if err != nil {
		return err
	}
	if _, err := a.svc.Users.Invite(r.Context(), me(r), id, in.Email); err != nil {
		return err
	}
	d, err := a.svc.Friends.Detail(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toFriend(d.Summary))
}

func (a *API) removeFriend(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "userId", "Friend")
	if err != nil {
		return err
	}
	if err := a.svc.Friends.Remove(r.Context(), me(r), id); err != nil {
		return err
	}
	return noContent(w)
}

func toPatch(p platform.GroupPatch) group.Patch {
	out := group.Patch{Name: p.Name, DefaultCurrency: p.DefaultCurrency, SimplifyDebts: p.SimplifyDebts, Emoji: p.Emoji, Color: p.Color, Archived: p.Archived}
	if p.Kind != nil {
		k := group.Kind(*p.Kind)
		out.Kind = &k
	}
	return out
}

func (a *API) listGroups(w http.ResponseWriter, r *http.Request) error {
	archived, _ := strconv.ParseBool(r.URL.Query().Get("archived"))
	groups, err := a.svc.Groups.List(r.Context(), me(r), archived)
	if err != nil {
		return err
	}
	out := make([]platform.Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, toGroup(g))
	}
	return list(w, out, len(out))
}

func (a *API) createGroup(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.GroupCreate](r)
	if err != nil {
		return err
	}
	create := group.CreateInput{Patch: toPatch(in.GroupPatch)}
	if create.ID, err = optionalID(in.ID, "id"); err != nil {
		return err
	}
	for _, m := range in.Members {
		create.Members = append(create.Members, group.Invitee{Email: m.Email, Name: m.Name})
	}
	v, err := a.svc.Groups.Create(r.Context(), me(r), create)
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, toGroup(v))
}

func (a *API) getGroup(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	v, err := a.svc.Groups.View(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toGroup(v))
}

func (a *API) patchGroup(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	in, err := decode[platform.GroupPatch](r)
	if err != nil {
		return err
	}
	v, err := a.svc.Groups.Update(r.Context(), me(r), id, toPatch(in))
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toGroup(v))
}

func (a *API) deleteGroup(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	if err := a.svc.Groups.Delete(r.Context(), me(r), id); err != nil {
		return err
	}
	return noContent(w)
}

func (a *API) addMember(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	in, err := decode[platform.Invitee](r)
	if err != nil {
		return err
	}
	v, err := a.svc.Groups.AddMember(r.Context(), me(r), id, group.Invitee{Email: in.Email, Name: in.Name})
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, toGroup(v))
}

func (a *API) removeMember(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	member, err := pathID(r, "userId", "Member")
	if err != nil {
		return err
	}
	if err := a.svc.Groups.RemoveMember(r.Context(), me(r), id, member); err != nil {
		return err
	}
	return noContent(w)
}

func (a *API) groupBalances(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	sheet, err := a.svc.Groups.Balances(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	out := platform.GroupBalances{Members: []platform.MemberBalance{}, Debts: toDebts(sheet.Debts)}
	for _, m := range sheet.Group.Members {
		out.Members = append(out.Members, platform.MemberBalance{UserID: m.String(), Balances: toBalances(sheet.Members[m])})
	}
	for id, b := range sheet.Members {
		if !sheet.Group.HasMember(id) && !b.Settled() {
			out.Members = append(out.Members, platform.MemberBalance{UserID: id.String(), Balances: toBalances(b)})
		}
	}
	return details(w, http.StatusOK, out)
}
