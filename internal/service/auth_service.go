package service

import (
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/google/uuid"
)

// NewSubscriberUser returns a new v2_user row with fresh node UUID and
// subscription token, as registration creates it.
func NewSubscriberUser(email, passwordHash string) *model.User {
	return &model.User{
		Email:    email,
		Password: passwordHash,
		UUID:     uuid.New().String(),
		Token:    uuid.New().String(),
		IsAdmin:  0,
		Banned:   0,
	}
}
