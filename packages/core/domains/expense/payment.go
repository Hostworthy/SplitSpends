package expense

import (
	"strconv"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// PaymentInput records money changing hands.
type PaymentInput struct {
	ID       *uuid.UUID
	GroupID  *uuid.UUID
	From     uuid.UUID
	To       uuid.UUID
	Amount   int64
	Currency string
	Date     string
	Notes    string
}

// payment validates a settle-up payment: one person pays another, and nobody
// else takes part. Payments are the only expenses in PaymentCategory, and
// both recording and editing one go through here.
func payment(in PaymentInput, today time.Time) (*Normalized, error) {
	if in.From == in.To || in.From == uuid.Nil || in.To == uuid.Nil {
		return nil, errs.Validation("toUserId", "Choose two different people.")
	}
	n, err := Normalize(Input{
		GroupID: in.GroupID, Description: "Payment", Currency: in.Currency, Total: in.Amount, Date: in.Date,
		Notes: in.Notes, Method: MethodExact,
		Payers: []Payer{{UserID: in.From, Amount: in.Amount}},
		Splits: []SplitInput{{UserID: in.To, Value: strconv.FormatInt(in.Amount, 10)}},
	}, today)
	if err != nil {
		if de, ok := errs.As(err); ok && de.Field == "total" {
			de.Field = "amount"
		}
		return nil, err
	}
	n.Category = PaymentCategory
	return n, nil
}

// repayment reads an edit of a payment, sent like any expense update: it
// must still have one payer paying the whole amount to one recipient.
func repayment(in Input) (PaymentInput, error) {
	if len(in.Payers) != 1 || len(in.Splits) != 1 {
		return PaymentInput{}, errs.Validation("splits", "A payment goes from one person to one other person.")
	}
	if in.Payers[0].Amount != in.Total {
		return PaymentInput{}, errs.Validation("payers", "Paid amounts must add up to the total.")
	}
	return PaymentInput{GroupID: in.GroupID, From: in.Payers[0].UserID, To: in.Splits[0].UserID, Amount: in.Total,
		Currency: in.Currency, Date: in.Date, Notes: in.Notes}, nil
}
