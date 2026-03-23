package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alekslesik/telegram-bot-simple/internal/repository"
)

const (
	StateWaitingService = "waiting_service"
	StateWaitingSlot    = "waiting_slot"
	StateWaitingConfirm = "waiting_confirm"
)

type statePayload struct {
	ServiceID int64 `json:"service_id"`
	SlotID    int64 `json:"slot_id"`
}

type BookingService struct {
	repo repository.BookingRepository
}

func NewBookingService(repo repository.BookingRepository) *BookingService {
	return &BookingService{repo: repo}
}

func (s *BookingService) Start(ctx context.Context, userID int64) (string, error) {
	services, err := s.repo.ListActiveServices(ctx)
	if err != nil {
		return "", err
	}
	if len(services) == 0 {
		return "No services available right now. Please try again later.", nil
	}
	if err := s.saveState(ctx, userID, StateWaitingService, statePayload{}); err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("Choose a service by number:\n")
	for i, srv := range services {
		fmt.Fprintf(&b, "%d) %s (%d min)\n", i+1, srv.Name, srv.DurationMin)
	}
	b.WriteString("\nSend /cancel to reset.")
	return strings.TrimSpace(b.String()), nil
}

func (s *BookingService) Cancel(ctx context.Context, userID int64) (string, error) {
	if err := s.repo.DeleteConversationState(ctx, userID); err != nil {
		return "", err
	}
	return "Booking flow cancelled. Send /book to start again.", nil
}

func (s *BookingService) HandleText(ctx context.Context, userID int64, text string) (bool, string, error) {
	state, payload, err := s.loadState(ctx, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return false, "", nil
		}
		return false, "", err
	}

	switch state {
	case StateWaitingService:
		return s.handleServiceSelection(ctx, userID, payload, text)
	case StateWaitingSlot:
		return s.handleSlotSelection(ctx, userID, payload, text)
	case StateWaitingConfirm:
		return s.handleConfirmation(ctx, userID, payload, text)
	default:
		return false, "", nil
	}
}

func (s *BookingService) handleServiceSelection(ctx context.Context, userID int64, payload statePayload, text string) (bool, string, error) {
	services, err := s.repo.ListActiveServices(ctx)
	if err != nil {
		return true, "", err
	}

	choice, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || choice < 1 || choice > len(services) {
		return true, "Please send a valid service number from the list.", nil
	}

	selected := services[choice-1]
	payload.ServiceID = selected.ID
	if err := s.saveState(ctx, userID, StateWaitingSlot, payload); err != nil {
		return true, "", err
	}

	slots, err := s.repo.ListAvailableSlots(ctx, selected.ID)
	if err != nil {
		return true, "", err
	}
	if len(slots) == 0 {
		return true, "No available slots for this service. Send /book to pick another service.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Service selected: %s\n\nChoose a slot by number:\n", selected.Name)
	for i, slot := range slots {
		fmt.Fprintf(&b, "%d) %s\n", i+1, slot.StartAt.Format("2006-01-02 15:04"))
	}
	return true, strings.TrimSpace(b.String()), nil
}

func (s *BookingService) handleSlotSelection(ctx context.Context, userID int64, payload statePayload, text string) (bool, string, error) {
	slots, err := s.repo.ListAvailableSlots(ctx, payload.ServiceID)
	if err != nil {
		return true, "", err
	}
	if len(slots) == 0 {
		return true, "No available slots right now. Send /book to restart.", nil
	}

	choice, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || choice < 1 || choice > len(slots) {
		return true, "Please send a valid slot number from the list.", nil
	}

	selectedSlot := slots[choice-1]
	payload.SlotID = selectedSlot.ID
	if err := s.saveState(ctx, userID, StateWaitingConfirm, payload); err != nil {
		return true, "", err
	}

	service, err := s.repo.GetServiceByID(ctx, payload.ServiceID)
	if err != nil {
		return true, "", err
	}
	return true, fmt.Sprintf(
		"Confirm booking:\nService: %s\nSlot: %s\n\nReply YES to confirm or NO to cancel.",
		service.Name,
		selectedSlot.StartAt.Format("2006-01-02 15:04"),
	), nil
}

func (s *BookingService) handleConfirmation(ctx context.Context, userID int64, payload statePayload, text string) (bool, string, error) {
	decision := strings.ToUpper(strings.TrimSpace(text))
	switch decision {
	case "NO", "N", "CANCEL":
		if err := s.repo.DeleteConversationState(ctx, userID); err != nil {
			return true, "", err
		}
		return true, "Booking cancelled. Send /book to start again.", nil
	case "YES", "Y":
		// continue below
	default:
		return true, "Please reply YES or NO.", nil
	}

	if err := s.repo.MarkSlotUnavailable(ctx, payload.SlotID); err != nil {
		return true, "", err
	}
	booking, err := s.repo.CreateBooking(ctx, repository.Booking{
		TelegramUserID: userID,
		ServiceID:      payload.ServiceID,
		SlotID:         payload.SlotID,
		Status:         "confirmed",
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		return true, "", err
	}
	if err := s.repo.DeleteConversationState(ctx, userID); err != nil {
		return true, "", err
	}
	return true, fmt.Sprintf("Booking confirmed. ID: %d", booking.ID), nil
}

func (s *BookingService) loadState(ctx context.Context, userID int64) (string, statePayload, error) {
	st, err := s.repo.GetConversationState(ctx, userID)
	if err != nil {
		return "", statePayload{}, err
	}
	payload := statePayload{}
	if st.PayloadJSON != "" {
		if err := json.Unmarshal([]byte(st.PayloadJSON), &payload); err != nil {
			return "", statePayload{}, err
		}
	}
	return st.State, payload, nil
}

func (s *BookingService) saveState(ctx context.Context, userID int64, state string, payload statePayload) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.repo.SaveConversationState(ctx, repository.ConversationState{
		TelegramUserID: userID,
		State:          state,
		PayloadJSON:    string(raw),
		UpdatedAt:      time.Now().UTC(),
	})
}
