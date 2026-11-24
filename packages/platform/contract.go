// Package platform is the HTTP contract: request and response shapes shared by
// the API server, the MCP server and Go clients. It depends only on the
// standard library so any transport can use it.
package platform

import "encoding/json"

// Prefix is the versioned API root.
const Prefix = "/api/v1"

// Details wraps a single object.
type Details[T any] struct {
	Data T `json:"data"`
}

// List wraps a page of objects.
type List[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// Error is the body of every failed response.
type Error struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	Field     string `json:"field,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// Balance is an amount in minor units of Currency.
type Balance struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

// Debt says From owes To.
type Debt struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

// User is a public profile.
type User struct {
	ID string `json:"id"`
	// Email is null for people added by name only.
	Email           *string `json:"email"`
	Name            string  `json:"name"`
	DefaultCurrency string  `json:"defaultCurrency"`
	AvatarColor     string  `json:"avatarColor"`
	Status          string  `json:"status"`
}

// Notifications are email preferences.
type Notifications struct {
	ExpenseAdded    bool `json:"expenseAdded"`
	ExpenseUpdated  bool `json:"expenseUpdated"`
	PaymentReceived bool `json:"paymentReceived"`
	AddedToGroup    bool `json:"addedToGroup"`
}

// Me is the caller's own profile.
type Me struct {
	User
	Timezone      string        `json:"timezone"`
	Notifications Notifications `json:"notifications"`
}

// MePatch updates the caller's profile.
type MePatch struct {
	Name            *string        `json:"name,omitempty"`
	DefaultCurrency *string        `json:"defaultCurrency,omitempty"`
	Timezone        *string        `json:"timezone,omitempty"`
	AvatarColor     *string        `json:"avatarColor,omitempty"`
	Notifications   *Notifications `json:"notifications,omitempty"`
}

// MyBalances are totals across everything.
type MyBalances struct {
	Balances []Balance `json:"balances"`
	Owed     []Balance `json:"owed"`
	Owing    []Balance `json:"owing"`
}

// Currency metadata.
type Currency struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Exponent int    `json:"exponent"`
}

// Category metadata.
type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
	Icon  string `json:"icon"`
}

// Config is public instance configuration.
type Config struct {
	AppName    string     `json:"appName"`
	AIEnabled  bool       `json:"aiEnabled"`
	SignupMode string     `json:"signupMode"`
	Currencies []Currency `json:"currencies"`
	Categories []Category `json:"categories"`
}

// OTPRequest asks for a login code.
type OTPRequest struct {
	Email string `json:"email"`
}

// OTPSent confirms a code request.
type OTPSent struct {
	Sent             bool `json:"sent"`
	ExpiresInSeconds int  `json:"expiresInSeconds"`
}

// OTPVerify exchanges a code for a session.
type OTPVerify struct {
	Email string `json:"email"`
	Code  string `json:"code"`
	Name  string `json:"name,omitempty"`
}

// LoginResult is a new session.
type LoginResult struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
	User      Me     `json:"user"`
	IsNew     bool   `json:"isNew"`
}

// Token is a personal API token.
type Token struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Token      string  `json:"token,omitempty"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
}

// TokenCreate names a new token.
type TokenCreate struct {
	Name string `json:"name"`
}

// Invitee identifies a person by email, or by name only when email is empty.
type Invitee struct {
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
}

// FriendInvite attaches an email to someone added by name only.
type FriendInvite struct {
	Email string `json:"email"`
}

// Friend is a friend and the balance with them.
type Friend struct {
	User     User      `json:"user"`
	Balances []Balance `json:"balances"`
}

// FriendGroup is the balance with a friend inside one group.
type FriendGroup struct {
	GroupID   string    `json:"groupId"`
	GroupName string    `json:"groupName"`
	Balances  []Balance `json:"balances"`
}

// FriendDetail breaks a friend balance down.
type FriendDetail struct {
	Friend
	Groups   []FriendGroup `json:"groups"`
	NonGroup []Balance     `json:"nonGroup"`
}

// Group is a group with members and the caller's balance.
type Group struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Kind            string    `json:"kind"`
	DefaultCurrency string    `json:"defaultCurrency"`
	SimplifyDebts   bool      `json:"simplifyDebts"`
	Emoji           string    `json:"emoji"`
	Color           string    `json:"color"`
	Archived        bool      `json:"archived"`
	CreatedAt       string    `json:"createdAt"`
	UpdatedAt       string    `json:"updatedAt"`
	Members         []User    `json:"members"`
	Balances        []Balance `json:"balances"`
}

// GroupCreate creates a group.
type GroupCreate struct {
	ID *string `json:"id,omitempty"`
	GroupPatch
	Members []Invitee `json:"members,omitempty"`
}

// GroupPatch updates a group.
type GroupPatch struct {
	Name            *string `json:"name,omitempty"`
	Kind            *string `json:"kind,omitempty"`
	DefaultCurrency *string `json:"defaultCurrency,omitempty"`
	SimplifyDebts   *bool   `json:"simplifyDebts,omitempty"`
	Emoji           *string `json:"emoji,omitempty"`
	Color           *string `json:"color,omitempty"`
	Archived        *bool   `json:"archived,omitempty"`
}

// MemberBalance is one member's balance in a group.
type MemberBalance struct {
	UserID   string    `json:"userId"`
	Balances []Balance `json:"balances"`
}

// GroupBalances is a group's balance sheet.
type GroupBalances struct {
	Members []MemberBalance `json:"members"`
	Debts   []Debt          `json:"debts"`
}

// Share is a participant's position in an expense.
type Share struct {
	UserID string       `json:"userId"`
	Paid   int64        `json:"paid"`
	Owed   int64        `json:"owed"`
	Value  *json.Number `json:"value"`
}

// Item is a line of an itemized expense.
type Item struct {
	Name    string   `json:"name"`
	Amount  int64    `json:"amount"`
	UserIDs []string `json:"userIds"`
}

// Expense is an expense or payment.
type Expense struct {
	ID           string  `json:"id"`
	GroupID      *string `json:"groupId"`
	Kind         string  `json:"kind"`
	Description  string  `json:"description"`
	Category     string  `json:"category"`
	Currency     string  `json:"currency"`
	Total        int64   `json:"total"`
	Date         string  `json:"date"`
	Notes        string  `json:"notes"`
	SplitMethod  string  `json:"splitMethod"`
	Shares       []Share `json:"shares"`
	Items        []Item  `json:"items"`
	Tax          int64   `json:"tax"`
	Tip          int64   `json:"tip"`
	ReceiptID    *string `json:"receiptId"`
	Recurrence   string  `json:"recurrence"`
	NextDate     *string `json:"nextDate"`
	CreatedBy    string  `json:"createdBy"`
	UpdatedBy    string  `json:"updatedBy"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
	Deleted      bool    `json:"deleted"`
	CommentCount int     `json:"commentCount"`
	MyBalance    int64   `json:"myBalance"`
}

// Payer says how much one user paid.
type Payer struct {
	UserID string `json:"userId"`
	Amount int64  `json:"amount"`
}

// Split is one participant's split input.
type Split struct {
	UserID string       `json:"userId"`
	Value  *json.Number `json:"value,omitempty"`
}

// ExpenseInput creates or replaces an expense.
type ExpenseInput struct {
	ID          *string `json:"id,omitempty"`
	GroupID     *string `json:"groupId,omitempty"`
	Description string  `json:"description"`
	Currency    string  `json:"currency"`
	Total       int64   `json:"total"`
	Date        string  `json:"date,omitempty"`
	Category    string  `json:"category,omitempty"`
	Notes       string  `json:"notes,omitempty"`
	SplitMethod string  `json:"splitMethod"`
	Payers      []Payer `json:"payers"`
	Splits      []Split `json:"splits,omitempty"`
	Items       []Item  `json:"items,omitempty"`
	Tax         int64   `json:"tax,omitempty"`
	Tip         int64   `json:"tip,omitempty"`
	ReceiptID   *string `json:"receiptId,omitempty"`
	Recurrence  string  `json:"recurrence,omitempty"`
}

// PaymentInput records a settle-up payment.
type PaymentInput struct {
	ID         *string `json:"id,omitempty"`
	GroupID    *string `json:"groupId,omitempty"`
	FromUserID string  `json:"fromUserId"`
	ToUserID   string  `json:"toUserId"`
	Amount     int64   `json:"amount"`
	Currency   string  `json:"currency"`
	Date       string  `json:"date,omitempty"`
	Notes      string  `json:"notes,omitempty"`
}

// Comment is a note on an expense.
type Comment struct {
	ID        string `json:"id"`
	ExpenseID string `json:"expenseId"`
	User      User   `json:"user"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

// CommentInput adds a comment.
type CommentInput struct {
	Body string `json:"body"`
}

// Activity is a feed event.
type Activity struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Actor     User           `json:"actor"`
	GroupID   *string        `json:"groupId"`
	ExpenseID *string        `json:"expenseId"`
	Summary   string         `json:"summary"`
	Data      map[string]any `json:"data"`
	CreatedAt string         `json:"createdAt"`
}

// Amount is a labelled total.
type CategoryAmount struct {
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
}

// MonthAmount is a monthly total.
type MonthAmount struct {
	Month  string `json:"month"`
	Amount int64  `json:"amount"`
}

// MemberSpend is what a member paid and owed.
type MemberSpend struct {
	UserID string `json:"userId"`
	Paid   int64  `json:"paid"`
	Owed   int64  `json:"owed"`
}

// Stats is chart data.
type Stats struct {
	Currency   string           `json:"currency"`
	TotalSpent int64            `json:"totalSpent"`
	ByCategory []CategoryAmount `json:"byCategory"`
	ByMonth    []MonthAmount    `json:"byMonth"`
	ByMember   []MemberSpend    `json:"byMember"`
}

// Receipt is an uploaded image.
type Receipt struct {
	ID          string `json:"id"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	CreatedAt   string `json:"createdAt"`
}

// ScanRequest asks to read a receipt.
type ScanRequest struct {
	Currency string `json:"currency,omitempty"`
}

// ScanItem is a receipt line.
type ScanItem struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Amount   int64  `json:"amount"`
}

// ReceiptScan is a read receipt.
type ReceiptScan struct {
	Merchant string     `json:"merchant"`
	Date     *string    `json:"date"`
	Currency string     `json:"currency"`
	Items    []ScanItem `json:"items"`
	Subtotal int64      `json:"subtotal"`
	Tax      int64      `json:"tax"`
	Tip      int64      `json:"tip"`
	Total    int64      `json:"total"`
	Category string     `json:"category"`
}

// DraftRequest asks the configured AI provider to turn instructions, and
// optionally an uploaded receipt, into an expense draft.
type DraftRequest struct {
	Text      string  `json:"text"`
	GroupID   *string `json:"groupId,omitempty"`
	ReceiptID *string `json:"receiptId,omitempty"`
}

// ExpenseDraft is a reviewable expense body (a POST /expenses body plus
// warnings). It is never saved automatically.
type ExpenseDraft struct {
	GroupID     *string  `json:"groupId"`
	ReceiptID   *string  `json:"receiptId"`
	Description string   `json:"description"`
	Currency    string   `json:"currency"`
	Total       int64    `json:"total"`
	Date        string   `json:"date"`
	Category    string   `json:"category"`
	SplitMethod string   `json:"splitMethod"`
	Payers      []Payer  `json:"payers"`
	Splits      []Split  `json:"splits"`
	Items       []Item   `json:"items"`
	Tax         int64    `json:"tax"`
	Tip         int64    `json:"tip"`
	Warnings    []string `json:"warnings"`
}

// Health is the liveness response.
type Health struct {
	OK bool `json:"ok"`
}

// ImportPerson is someone from the device's local address book.
type ImportPerson struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// ImportGroup is a local group; memberIds are local person ids.
type ImportGroup struct {
	ID string `json:"id"`
	GroupPatch
	MemberIDs []string `json:"memberIds"`
}

// ImportExpense is a local expense or payment using local person ids.
type ImportExpense struct {
	Kind string `json:"kind,omitempty"`
	ExpenseInput
}

// ImportRequest uploads local-only data to an account. meId is the local
// person id that stands for the device owner.
type ImportRequest struct {
	MeID     string          `json:"meId"`
	People   []ImportPerson  `json:"people"`
	Groups   []ImportGroup   `json:"groups"`
	Expenses []ImportExpense `json:"expenses"`
}

// ImportResult maps local person ids to server user ids.
type ImportResult struct {
	People   map[string]string `json:"people"`
	Groups   int               `json:"groups"`
	Expenses int               `json:"expenses"`
}
