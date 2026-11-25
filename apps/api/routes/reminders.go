package routes

import (
	"net/http"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/reminder"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func (a *API) sendReminder(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.ReminderInput](w, r)
	if err != nil {
		return err
	}
	to, err := requiredID(in.UserID, "userId")
	if err != nil {
		return err
	}
	groupID, err := optionalID(in.GroupID, "groupId")
	if err != nil {
		return err
	}
	sent, err := a.svc.Reminders.Send(r.Context(), me(r), to, groupID)
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, toReminder(sent))
}

func toReminder(rm *reminder.Reminder) platform.Reminder {
	return platform.Reminder{ID: rm.ID.String(), UserID: rm.ToID.String(), GroupID: idPtr(rm.GroupID),
		Balances: toBalances(rm.Owed), Emailed: rm.Emailed, SentAt: stamp(rm.SentAt)}
}
