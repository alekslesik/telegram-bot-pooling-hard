package repository

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")

type Service struct {
	ID          int64
	Name        string
	DurationMin int
	IsActive    bool
}

type Slot struct {
	ID          int64
	ServiceID   int64
	StartAt     time.Time
	IsAvailable bool
}

type Booking struct {
	ID             int64
	TelegramUserID int64
	ServiceID      int64
	SlotID         int64
	Status         string
	CreatedAt      time.Time
}

type ConversationState struct {
	TelegramUserID int64
	State          string
	PayloadJSON    string
	UpdatedAt      time.Time
}

type Client struct {
	TelegramUserID int64
	FullName       string
	Phone          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type BookingRepository interface {
	ListActiveServices(ctx context.Context) ([]Service, error)
	GetServiceByID(ctx context.Context, serviceID int64) (Service, error)
	ListAvailableSlots(ctx context.Context, serviceID int64) ([]Slot, error)
	GetSlotByID(ctx context.Context, slotID int64) (Slot, error)
	GetClientByUserID(ctx context.Context, userID int64) (Client, error)
	UpsertClient(ctx context.Context, client Client) (Client, error)
	CreateBooking(ctx context.Context, booking Booking) (Booking, error)
	MarkSlotUnavailable(ctx context.Context, slotID int64) error
	GetConversationState(ctx context.Context, userID int64) (ConversationState, error)
	SaveConversationState(ctx context.Context, state ConversationState) error
	DeleteConversationState(ctx context.Context, userID int64) error
}

type MemoryRepository struct {
	mu            sync.RWMutex
	services      map[int64]Service
	slots         map[int64]Slot
	bookings      map[int64]Booking
	states        map[int64]ConversationState
	clients       map[int64]Client
	nextBookingID int64
	nextServiceID int64
	nextSlotID    int64
}

func NewMemoryRepository() *MemoryRepository {
	r := &MemoryRepository{
		services:      make(map[int64]Service),
		slots:         make(map[int64]Slot),
		bookings:      make(map[int64]Booking),
		states:        make(map[int64]ConversationState),
		clients:       make(map[int64]Client),
		nextBookingID: 1,
		nextServiceID: 1,
		nextSlotID:    1,
	}
	r.seed()
	return r
}

func (r *MemoryRepository) seed() {
	now := time.Now().Truncate(time.Hour)
	for _, item := range []struct {
		name     string
		duration int
	}{
		{name: "Haircut", duration: 60},
		{name: "Consultation", duration: 30},
	} {
		service := Service{
			ID:          r.nextServiceID,
			Name:        item.name,
			DurationMin: item.duration,
			IsActive:    true,
		}
		r.services[service.ID] = service
		r.nextServiceID++
		for i := 1; i <= 3; i++ {
			slot := Slot{
				ID:          r.nextSlotID,
				ServiceID:   service.ID,
				StartAt:     now.Add(time.Duration(i*2) * time.Hour),
				IsAvailable: true,
			}
			r.slots[slot.ID] = slot
			r.nextSlotID++
		}
	}
}

func (r *MemoryRepository) ListActiveServices(_ context.Context) ([]Service, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Service
	for _, service := range r.services {
		if service.IsActive {
			out = append(out, service)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *MemoryRepository) GetServiceByID(_ context.Context, serviceID int64) (Service, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	service, ok := r.services[serviceID]
	if !ok {
		return Service{}, ErrNotFound
	}
	return service, nil
}

func (r *MemoryRepository) ListAvailableSlots(_ context.Context, serviceID int64) ([]Slot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Slot
	for _, slot := range r.slots {
		if slot.ServiceID == serviceID && slot.IsAvailable {
			out = append(out, slot)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartAt.Before(out[j].StartAt) })
	return out, nil
}

func (r *MemoryRepository) GetSlotByID(_ context.Context, slotID int64) (Slot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	slot, ok := r.slots[slotID]
	if !ok {
		return Slot{}, ErrNotFound
	}
	return slot, nil
}

func (r *MemoryRepository) CreateBooking(_ context.Context, booking Booking) (Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	booking.ID = r.nextBookingID
	r.nextBookingID++
	if booking.CreatedAt.IsZero() {
		booking.CreatedAt = time.Now().UTC()
	}
	r.bookings[booking.ID] = booking
	return booking, nil
}

func (r *MemoryRepository) MarkSlotUnavailable(_ context.Context, slotID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	slot, ok := r.slots[slotID]
	if !ok {
		return ErrNotFound
	}
	slot.IsAvailable = false
	r.slots[slotID] = slot
	return nil
}

func (r *MemoryRepository) GetConversationState(_ context.Context, userID int64) (ConversationState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.states[userID]
	if !ok {
		return ConversationState{}, ErrNotFound
	}
	return state, nil
}

func (r *MemoryRepository) GetClientByUserID(_ context.Context, userID int64) (Client, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.clients[userID]
	if !ok {
		return Client{}, ErrNotFound
	}
	return client, nil
}

func (r *MemoryRepository) UpsertClient(_ context.Context, client Client) (Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	existing, ok := r.clients[client.TelegramUserID]
	if ok {
		existing.FullName = client.FullName
		existing.Phone = client.Phone
		existing.UpdatedAt = now
		r.clients[client.TelegramUserID] = existing
		return existing, nil
	}
	if client.CreatedAt.IsZero() {
		client.CreatedAt = now
	}
	client.UpdatedAt = now
	r.clients[client.TelegramUserID] = client
	return client, nil
}

func (r *MemoryRepository) SaveConversationState(_ context.Context, state ConversationState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	r.states[state.TelegramUserID] = state
	return nil
}

func (r *MemoryRepository) DeleteConversationState(_ context.Context, userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.states, userID)
	return nil
}
