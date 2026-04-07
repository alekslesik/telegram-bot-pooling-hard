package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

type bookingOutboxPayload struct {
	BookingID int64 `json:"booking_id"`
	UserID    int64 `json:"user_id"`
	SlotID    int64 `json:"slot_id"`
}

func NewBookingOutboxHandler(repo repository.BookingRepository) OutboxHandler {
	return func(ctx context.Context, event repository.OutboxEvent) error {
		switch event.EventType {
		case "booking_confirmed":
			return handleBookingConfirmedEvent(ctx, repo, event)
		case "booking_reminder_due":
			return handleBookingReminderDueEvent(ctx, repo, event)
		default:
			return nil
		}
	}
}

func handleBookingConfirmedEvent(ctx context.Context, repo repository.BookingRepository, event repository.OutboxEvent) error {
	var payload bookingOutboxPayload
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("decode booking_confirmed payload: %w", err)
	}
	if payload.BookingID == 0 || payload.UserID == 0 || payload.SlotID == 0 {
		return fmt.Errorf("invalid booking_confirmed payload")
	}
	slot, err := repo.GetDoctorSlotByID(ctx, payload.SlotID)
	if err != nil {
		return err
	}

	dueAt := slot.StartAt.Add(-1 * time.Hour)
	if dueAt.Before(time.Now().UTC()) {
		dueAt = time.Now().UTC()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	bookingID := payload.BookingID
	_, err = repo.EnqueueOutboxEvent(ctx, repository.OutboxEvent{
		EventType:     "booking_reminder_due",
		AggregateType: "clinic_booking",
		AggregateID:   &bookingID,
		PayloadJSON:   string(raw),
		AvailableAt:   dueAt,
	})
	return err
}

func handleBookingReminderDueEvent(ctx context.Context, repo repository.BookingRepository, event repository.OutboxEvent) error {
	var payload bookingOutboxPayload
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("decode booking_reminder_due payload: %w", err)
	}
	if payload.UserID == 0 {
		return fmt.Errorf("invalid booking_reminder_due payload")
	}
	uid := payload.UserID
	return repo.LogAnalyticsEvent(ctx, &uid, "booking_reminder_sent", event.PayloadJSON)
}
