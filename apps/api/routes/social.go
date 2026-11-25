package routes

import (
	"net/http"
	"strconv"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
)

func (a *API) listFriends(w http.ResponseWriter, r *http.Request) error {
	friends, err := a.svc.Friends.List(r.Context(), me(r))
	if err != nil {
		return err
	}
	out := make([]platform.Friend, 0, len(friends))
	for _, f := range friends {
		out = append(out, a.toFriend(f))
	}
	return paged(w, r, out)
}

func (a *API) addFriend(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.Invitee](w, r)
	if err != nil {
		return err
	}
	f, err := a.svc.Friends.Add(r.Context(), me(r), in.Email, in.Name)
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, a.toFriend(*f))
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
	out := platform.FriendDetail{Friend: a.toFriend(d.Summary), Groups: []platform.FriendGroup{}, NonGroup: toBalances(d.NonGroup)}
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
	in, err := decode[platform.FriendInvite](w, r)
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
	return details(w, http.StatusOK, a.toFriend(d.Summary))
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

func toPatch(p platform.GroupPatch) (group.Patch, error) {
	out := group.Patch{Name: p.Name, DefaultCurrency: p.DefaultCurrency, SimplifyDebts: p.SimplifyDebts, Emoji: p.Emoji, Color: p.Color, Archived: p.Archived}
	if p.Kind != nil {
		k := group.Kind(*p.Kind)
		out.Kind = &k
	}
	if p.DefaultSplit.Set {
		// An empty method clears the default split.
		out.DefaultSplit = &group.DefaultSplit{}
		if d := p.DefaultSplit.Value; d != nil {
			out.DefaultSplit.Method = group.SplitMethod(d.Method)
			if d.Method == "" {
				return out, errs.Validation("defaultSplit", "A default split is equal, percent or shares.")
			}
			out.DefaultSplit.Values = make(map[uuid.UUID]string, len(d.Values))
			for raw, v := range d.Values {
				id, err := requiredID(raw, "defaultSplit")
				if err != nil {
					return out, err
				}
				out.DefaultSplit.Values[id] = v
			}
		}
	}
	return out, nil
}

func toDefaultSplit(d *group.DefaultSplit) *platform.DefaultSplit {
	if d == nil {
		return nil
	}
	out := &platform.DefaultSplit{Method: string(d.Method), Values: make(map[string]string, len(d.Values))}
	for id, v := range d.Values {
		out.Values[id.String()] = v
	}
	return out
}

func (a *API) listGroups(w http.ResponseWriter, r *http.Request) error {
	archived, _ := strconv.ParseBool(r.URL.Query().Get("archived"))
	limit, offset := window(r)
	groups, total, err := a.svc.Groups.List(r.Context(), me(r), archived, limit, offset)
	if err != nil {
		return err
	}
	out := make([]platform.Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, a.toGroup(g))
	}
	return list(w, out, total)
}

func (a *API) createGroup(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.GroupCreate](w, r)
	if err != nil {
		return err
	}
	create := group.CreateInput{}
	if create.Patch, err = toPatch(in.GroupPatch); err != nil {
		return err
	}
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
	return details(w, http.StatusCreated, a.toGroup(v))
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
	return details(w, http.StatusOK, a.toGroup(v))
}

func (a *API) patchGroup(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	in, err := decode[platform.GroupPatch](w, r)
	if err != nil {
		return err
	}
	patch, err := toPatch(in)
	if err != nil {
		return err
	}
	v, err := a.svc.Groups.Update(r.Context(), me(r), id, patch)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, a.toGroup(v))
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
	in, err := decode[platform.Invitee](w, r)
	if err != nil {
		return err
	}
	v, err := a.svc.Groups.AddMember(r.Context(), me(r), id, group.Invitee{Email: in.Email, Name: in.Name})
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, a.toGroup(v))
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
