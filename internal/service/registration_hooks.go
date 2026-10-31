package service

import (
	"errors"

	"gorm.io/gorm"
)

// InviteRedemption is an invite code that registration accepted. Consume
// marks it used by the new account and runs in the registration transaction.
type InviteRedemption struct {
	// InviterID is the account the code belongs to, when it has an owner.
	InviterID *uint
	Consume   func(tx *gorm.DB, userID uint) error
}

// InviteRefusal is an invite code that registration refuses; Message is
// shown to the registrant.
type InviteRefusal struct{ Message string }

func (e InviteRefusal) Error() string { return e.Message }

// InviteRedeemer checks an invite code inside the registration transaction.
type InviteRedeemer func(tx *gorm.DB, code string) (*InviteRedemption, error)

var inviteRedeemer InviteRedeemer

// RegisterInviteRedeemer installs the invite code feature. Call it from an
// init function: it is not guarded. Without one, a registration that carries
// an invite code is refused.
func RegisterInviteRedeemer(redeem InviteRedeemer) { inviteRedeemer = redeem }

// RedeemInviteCode checks code for a registration. An empty code redeems
// nothing and returns nil.
func RedeemInviteCode(tx *gorm.DB, code string) (*InviteRedemption, error) {
	if code == "" {
		return nil, nil
	}
	if inviteRedeemer == nil {
		return nil, InviteRefusal{"invite codes are not available"}
	}
	return inviteRedeemer(tx, code)
}

// IsInviteRefusal reports whether err is an InviteRefusal.
func IsInviteRefusal(err error) (InviteRefusal, bool) {
	var refusal InviteRefusal
	ok := errors.As(err, &refusal)
	return refusal, ok
}
