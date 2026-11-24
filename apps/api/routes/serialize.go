package routes

import (
	"encoding/json"
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

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func stampPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := stamp(*t)
	return &s
}

func idPtr(id *uuid.UUID) *string {
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

func toUser(u *user.User) platform.User {
	var email *string
	if u.Email != "" {
		email = &u.Email
	}
	return platform.User{ID: u.ID.String(), Email: email, Name: u.Name, DefaultCurrency: u.DefaultCurrency, AvatarColor: u.AvatarColor, Status: string(u.Status)}
}

func toMe(u *user.User) platform.Me {
	return platform.Me{User: toUser(u), Timezone: u.Timezone, Notifications: platform.Notifications{
		NotificationSwitches: platform.NotificationSwitches(u.Notifications.Switches),
		Push:                 platform.NotificationSwitches(u.Notifications.Push),
	}}
}

func toPushSubscription(s *push.Subscription) platform.PushSubscription {
	return platform.PushSubscription{ID: s.ID.String(), Platform: string(s.Platform), Label: s.Label, Endpoint: s.Endpoint,
		CreatedAt: stamp(s.CreatedAt), LastSuccessAt: stampPtr(s.LastSuccessAt), Failures: s.Failures}
}

func toToken(s *auth.Session, token string) platform.Token {
	return platform.Token{ID: s.ID.String(), Name: s.Name, Token: token, CreatedAt: stamp(s.CreatedAt), LastUsedAt: stampPtr(s.LastUsedAt)}
}

func toGroup(v *group.View) platform.Group {
	members := make([]platform.User, 0, len(v.People))
	for _, u := range v.People {
		members = append(members, toUser(u))
	}
	return platform.Group{ID: v.ID.String(), Name: v.Name, Kind: string(v.Kind), DefaultCurrency: v.DefaultCurrency,
		SimplifyDebts: v.SimplifyDebts, Emoji: v.Emoji, Color: v.Color, Archived: v.ArchivedAt != nil,
		CreatedAt: stamp(v.CreatedAt), UpdatedAt: stamp(v.UpdatedAt), Members: members, Balances: toBalances(v.Balances)}
}

func toDebts(ds []balance.Debt) []platform.Debt {
	out := make([]platform.Debt, 0, len(ds))
	for _, d := range ds {
		out = append(out, platform.Debt{From: d.From.String(), To: d.To.String(), Currency: d.Currency, Amount: d.Amount})
	}
	return out
}

func toFriend(f friend.Summary) platform.Friend {
	return platform.Friend{User: toUser(f.User), Balances: toBalances(f.Balances)}
}

func toExpense(e *expense.Expense, viewer uuid.UUID) platform.Expense {
	shares := make([]platform.Share, 0, len(e.Shares))
	for _, s := range e.Shares {
		var value *json.Number
		if s.Value != "" {
			v := json.Number(s.Value)
			value = &v
		}
		shares = append(shares, platform.Share{UserID: s.UserID.String(), Paid: s.Paid, Owed: s.Owed, Value: value})
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
	return platform.Expense{ID: e.ID.String(), GroupID: idPtr(e.GroupID), Kind: string(e.Kind), Description: e.Description,
		Category: e.Category, Currency: e.Currency, Total: e.Total, Date: e.Date.Format(time.DateOnly), Notes: e.Notes,
		SplitMethod: string(e.Method), Shares: shares, Items: items, Tax: e.Tax, Tip: e.Tip, ReceiptID: idPtr(e.ReceiptID),
		Recurrence: string(e.Recurrence), NextDate: next, CreatedBy: e.CreatedBy.String(), UpdatedBy: e.UpdatedBy.String(),
		CreatedAt: stamp(e.CreatedAt), UpdatedAt: stamp(e.UpdatedAt), Deleted: e.DeletedAt != nil, CommentCount: e.CommentCount,
		MyBalance: e.Net(viewer)}
}

func toActivity(a *activity.Activity, actor *user.User) platform.Activity {
	u := platform.User{ID: a.ActorID.String(), Name: "Someone"}
	if actor != nil {
		u = toUser(actor)
	}
	return platform.Activity{ID: a.ID.String(), Kind: string(a.Kind), Actor: u, GroupID: idPtr(a.GroupID), ExpenseID: idPtr(a.ExpenseID),
		Summary: activity.Describe(a, u.Name), Data: a.Data, CreatedAt: stamp(a.CreatedAt)}
}
