package routes

import (
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/balance"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/friend"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
)

// decimal returns a split value as decimal text. Values are stored as
// written, and plain decimals such as ".5", "1.", "+2" or "05" are not
// numbers every client can parse, so the value is written out again in
// plain form with the same decimal places.
func decimal(value string) *string {
	if value == "" {
		return nil
	}
	if r, ok := money.Decimal(value); ok {
		places := 0
		if i := strings.IndexByte(value, '.'); i >= 0 {
			places = len(value) - i - 1
		}
		value = r.FloatString(places)
	}
	return &value
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// optionalStamp is stamp for a time that may be unset, which stays null.
func optionalStamp(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := stamp(*t)
	return &s
}

// optionalString is an id that may be unset, as a string or null.
func optionalString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func toBalances(b money.Balances) []platform.Balance {
	out := []platform.Balance{}
	for _, x := range b.List() {
		out = append(out, platform.Balance{Currency: x.Currency, Amount: x.Amount})
	}
	return out
}

// toUser shows u to other people. Whether an administrator disabled someone
// is between them and the administrators, so others see them as active.
func (a *API) toUser(u *user.User) platform.User {
	out := a.toAccountUser(u)
	if u.Status == user.StatusDisabled {
		out.Status = string(user.StatusActive)
	}
	return out
}

// toAccountUser shows u with its real status, to itself and to
// administrators.
func (a *API) toAccountUser(u *user.User) platform.User {
	var email, picture *string
	if u.Email != "" {
		email = &u.Email
	}
	if p := a.svc.Users.Picture(u, 160); p != "" {
		picture = &p
	}
	return platform.User{ID: u.ID.String(), Email: email, Name: u.Name, DefaultCurrency: u.DefaultCurrency, AvatarColor: u.AvatarColor,
		AvatarURL: picture, Status: string(u.Status)}
}

func (a *API) toMe(u *user.User) platform.Me {
	return platform.Me{User: a.toAccountUser(u), Timezone: u.Timezone, Gravatar: u.Gravatar, Role: string(u.Role), Notifications: platform.Notifications{
		NotificationSwitches: platform.NotificationSwitches(u.Notifications.Switches),
		Push:                 platform.NotificationSwitches(u.Notifications.Push),
	}}
}

func toPushSubscription(s *push.Subscription) platform.PushSubscription {
	return platform.PushSubscription{ID: s.ID.String(), Platform: string(s.Platform), Label: s.Label, Endpoint: s.Endpoint,
		CreatedAt: stamp(s.CreatedAt), LastSuccessAt: optionalStamp(s.LastSuccessAt), Failures: s.Failures}
}

func toToken(s *auth.Session, token string) platform.Token {
	return platform.Token{ID: s.ID.String(), Name: s.Name, Token: token, CreatedAt: stamp(s.CreatedAt), LastUsedAt: optionalStamp(s.LastUsedAt)}
}

func (a *API) toGroup(v *group.View) platform.Group {
	members := make([]platform.User, 0, len(v.People))
	for _, u := range v.People {
		members = append(members, a.toUser(u))
	}
	return platform.Group{ID: v.ID.String(), Name: v.Name, Kind: string(v.Kind), DefaultCurrency: v.DefaultCurrency,
		SimplifyDebts: v.SimplifyDebts, Emoji: v.Emoji, Color: v.Color, Archived: v.ArchivedAt != nil,
		CreatedAt: stamp(v.CreatedAt), UpdatedAt: stamp(v.UpdatedAt), Members: members, Balances: toBalances(v.Balances),
		DefaultSplit: toDefaultSplit(v.Effective())}
}

func toDebts(ds []balance.Debt) []platform.Debt {
	out := make([]platform.Debt, 0, len(ds))
	for _, d := range ds {
		out = append(out, platform.Debt{From: d.From.String(), To: d.To.String(), Currency: d.Currency, Amount: d.Amount})
	}
	return out
}

func (a *API) toFriend(f friend.Summary) platform.Friend {
	return platform.Friend{User: a.toUser(f.User), Balances: toBalances(f.Balances)}
}

func toExpense(e *expense.Expense, viewer uuid.UUID) platform.Expense {
	shares := make([]platform.Share, 0, len(e.Shares))
	for _, s := range e.Shares {
		shares = append(shares, platform.Share{UserID: s.UserID.String(), Paid: s.Paid, Owed: s.Owed, Value: decimal(s.Value)})
	}
	items := make([]platform.Item, 0, len(e.Items))
	for _, it := range e.Items {
		ids := make([]string, len(it.UserIDs))
		for i, id := range it.UserIDs {
			ids[i] = id.String()
		}
		items = append(items, platform.Item{Name: it.Name, Amount: it.Amount, UserIDs: ids})
	}
	var next *string
	if e.NextDate != nil {
		s := e.NextDate.Format(time.DateOnly)
		next = &s
	}
	return platform.Expense{ID: e.ID.String(), GroupID: optionalString(e.GroupID), Kind: string(e.Kind), Description: e.Description,
		Category: e.Category, Currency: e.Currency, Total: e.Total, Date: e.Date.Format(time.DateOnly), Notes: e.Notes,
		SplitMethod: string(e.Method), Shares: shares, Items: items, Tax: e.Tax, Tip: e.Tip, ReceiptID: optionalString(e.ReceiptID),
		Recurrence: string(e.Recurrence), NextDate: next, CreatedBy: e.CreatedBy.String(), UpdatedBy: e.UpdatedBy.String(),
		CreatedAt: stamp(e.CreatedAt), UpdatedAt: stamp(e.UpdatedAt), Deleted: e.DeletedAt != nil, CommentCount: e.CommentCount,
		MyBalance: e.Net(viewer)}
}

func (a *API) toActivity(e *activity.Activity, actor *user.User) platform.Activity {
	u := platform.User{ID: e.ActorID.String(), Name: "Someone"}
	if actor != nil {
		u = a.toUser(actor)
	}
	return platform.Activity{ID: e.ID.String(), Kind: string(e.Kind), Actor: u, GroupID: optionalString(e.GroupID), ExpenseID: optionalString(e.ExpenseID),
		Summary: activity.Describe(e, u.Name), Data: e.Data, CreatedAt: stamp(e.CreatedAt)}
}
