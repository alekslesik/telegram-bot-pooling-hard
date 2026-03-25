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

type Specialty struct {
	ID        int64
	Name      string
	SortOrder int
	IsActive  bool
}

type Doctor struct {
	ID       int64
	FullName string
	IsActive bool
}

type DoctorSlot struct {
	ID          int64
	DoctorID    int64
	SpecialtyID int64
	StartAt     time.Time
	IsAvailable bool
}

type ClinicBooking struct {
	ID             int64
	TelegramUserID int64
	SpecialtyID    int64
	DoctorID       int64
	DoctorSlotID   int64
	Status         string
	CreatedAt      time.Time
	CancelledAt    *time.Time
}

type ClinicBookingView struct {
	ID            int64
	SpecialtyName string
	DoctorName    string
	StartAt       time.Time
	Status        string
	CreatedAt     time.Time
}

type BookingRepository interface {
	ListActiveServices(ctx context.Context) ([]Service, error)
	GetServiceByID(ctx context.Context, serviceID int64) (Service, error)
	ListAvailableSlots(ctx context.Context, serviceID int64) ([]Slot, error)
	GetSlotByID(ctx context.Context, slotID int64) (Slot, error)
	GetClientByUserID(ctx context.Context, userID int64) (Client, error)
	UpsertClient(ctx context.Context, client Client) (Client, error)
	ListSpecialties(ctx context.Context, limit, offset int) ([]Specialty, error)
	CountSpecialties(ctx context.Context) (int, error)
	GetSpecialtyByID(ctx context.Context, specialtyID int64) (Specialty, error)
	ListDoctorsBySpecialty(ctx context.Context, specialtyID int64, limit, offset int) ([]Doctor, error)
	CountDoctorsBySpecialty(ctx context.Context, specialtyID int64) (int, error)
	GetDoctorByID(ctx context.Context, doctorID int64) (Doctor, error)
	ListAvailableDoctorSlots(ctx context.Context, specialtyID, doctorID int64, limit, offset int) ([]DoctorSlot, error)
	CountAvailableDoctorSlots(ctx context.Context, specialtyID, doctorID int64) (int, error)
	GetDoctorSlotByID(ctx context.Context, slotID int64) (DoctorSlot, error)
	CreateBooking(ctx context.Context, booking Booking) (Booking, error)
	MarkSlotUnavailable(ctx context.Context, slotID int64) error
	CreateClinicBooking(ctx context.Context, booking ClinicBooking) (ClinicBooking, error)
	MarkDoctorSlotUnavailable(ctx context.Context, slotID int64) error
	ListUserClinicBookings(ctx context.Context, userID int64, limit, offset int) ([]ClinicBookingView, error)
	CountUserClinicBookings(ctx context.Context, userID int64) (int, error)
	CancelClinicBooking(ctx context.Context, userID, bookingID int64) (ClinicBookingView, error)
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
	specialties   map[int64]Specialty
	doctors       map[int64]Doctor
	doctorLinks   map[int64]map[int64]struct{}
	doctorSlots   map[int64]DoctorSlot
	clinicBooking map[int64]ClinicBooking
	nextBookingID int64
	nextServiceID int64
	nextSlotID    int64
	nextClinicID  int64
}

func NewMemoryRepository() *MemoryRepository {
	r := &MemoryRepository{
		services:      make(map[int64]Service),
		slots:         make(map[int64]Slot),
		bookings:      make(map[int64]Booking),
		states:        make(map[int64]ConversationState),
		clients:       make(map[int64]Client),
		specialties:   make(map[int64]Specialty),
		doctors:       make(map[int64]Doctor),
		doctorLinks:   make(map[int64]map[int64]struct{}),
		doctorSlots:   make(map[int64]DoctorSlot),
		clinicBooking: make(map[int64]ClinicBooking),
		nextBookingID: 1,
		nextServiceID: 1,
		nextSlotID:    1,
		nextClinicID:  1,
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

	r.specialties[1] = Specialty{ID: 1, Name: "Терапевт", SortOrder: 1, IsActive: true}
	r.specialties[2] = Specialty{ID: 2, Name: "Кардиолог", SortOrder: 2, IsActive: true}
	r.specialties[3] = Specialty{ID: 3, Name: "ЛОР", SortOrder: 3, IsActive: true}
	r.specialties[4] = Specialty{ID: 4, Name: "Невролог", SortOrder: 4, IsActive: true}

	r.doctors[1] = Doctor{ID: 1, FullName: "Иванов И.И.", IsActive: true}
	r.doctors[2] = Doctor{ID: 2, FullName: "Петрова А.С.", IsActive: true}
	r.doctors[3] = Doctor{ID: 3, FullName: "Смирнов Д.К.", IsActive: true}
	r.doctorLinks[1] = map[int64]struct{}{1: {}, 4: {}}
	r.doctorLinks[2] = map[int64]struct{}{2: {}}
	r.doctorLinks[3] = map[int64]struct{}{1: {}, 3: {}}

	for i := int64(1); i <= 3; i++ {
		start := now.Add(time.Duration(i*24) * time.Hour).Add(10 * time.Hour)
		r.doctorSlots[i] = DoctorSlot{ID: i, DoctorID: 1, SpecialtyID: 1, StartAt: start, IsAvailable: true}
		r.doctorSlots[i+10] = DoctorSlot{ID: i + 10, DoctorID: 2, SpecialtyID: 2, StartAt: start.Add(2 * time.Hour), IsAvailable: true}
		r.doctorSlots[i+20] = DoctorSlot{ID: i + 20, DoctorID: 3, SpecialtyID: 3, StartAt: start.Add(4 * time.Hour), IsAvailable: true}
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

func (r *MemoryRepository) CreateClinicBooking(_ context.Context, booking ClinicBooking) (ClinicBooking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	booking.ID = r.nextClinicID
	r.nextClinicID++
	if booking.CreatedAt.IsZero() {
		booking.CreatedAt = time.Now().UTC()
	}
	r.clinicBooking[booking.ID] = booking
	return booking, nil
}

func (r *MemoryRepository) MarkDoctorSlotUnavailable(_ context.Context, slotID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	slot, ok := r.doctorSlots[slotID]
	if !ok || !slot.IsAvailable {
		return ErrNotFound
	}
	slot.IsAvailable = false
	r.doctorSlots[slotID] = slot
	return nil
}

func (r *MemoryRepository) ListUserClinicBookings(_ context.Context, userID int64, limit, offset int) ([]ClinicBookingView, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []ClinicBookingView
	for _, b := range r.clinicBooking {
		if b.TelegramUserID != userID {
			continue
		}
		out = append(out, r.toClinicBookingViewLocked(b))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartAt.Before(out[j].StartAt) })
	start, end := pageBounds(len(out), limit, offset)
	return append([]ClinicBookingView(nil), out[start:end]...), nil
}

func (r *MemoryRepository) CountUserClinicBookings(_ context.Context, userID int64) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, b := range r.clinicBooking {
		if b.TelegramUserID == userID {
			count++
		}
	}
	return count, nil
}

func (r *MemoryRepository) CancelClinicBooking(_ context.Context, userID, bookingID int64) (ClinicBookingView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.clinicBooking[bookingID]
	if !ok || b.TelegramUserID != userID {
		return ClinicBookingView{}, ErrNotFound
	}
	if b.Status == "cancelled" {
		return r.toClinicBookingViewLocked(b), nil
	}
	slot, ok := r.doctorSlots[b.DoctorSlotID]
	if ok {
		slot.IsAvailable = true
		r.doctorSlots[b.DoctorSlotID] = slot
	}
	now := time.Now().UTC()
	b.Status = "cancelled"
	b.CancelledAt = &now
	r.clinicBooking[bookingID] = b
	return r.toClinicBookingViewLocked(b), nil
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

func (r *MemoryRepository) ListSpecialties(_ context.Context, limit, offset int) ([]Specialty, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Specialty, 0, len(r.specialties))
	for _, s := range r.specialties {
		if s.IsActive {
			items = append(items, s)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder == items[j].SortOrder {
			return items[i].ID < items[j].ID
		}
		return items[i].SortOrder < items[j].SortOrder
	})
	return pageSpecialties(items, limit, offset), nil
}

func (r *MemoryRepository) CountSpecialties(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, s := range r.specialties {
		if s.IsActive {
			count++
		}
	}
	return count, nil
}

func (r *MemoryRepository) GetSpecialtyByID(_ context.Context, specialtyID int64) (Specialty, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.specialties[specialtyID]
	if !ok {
		return Specialty{}, ErrNotFound
	}
	return s, nil
}

func (r *MemoryRepository) ListDoctorsBySpecialty(_ context.Context, specialtyID int64, limit, offset int) ([]Doctor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Doctor
	for doctorID, links := range r.doctorLinks {
		if _, ok := links[specialtyID]; !ok {
			continue
		}
		doc, ok := r.doctors[doctorID]
		if ok && doc.IsActive {
			out = append(out, doc)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName < out[j].FullName })
	return pageDoctors(out, limit, offset), nil
}

func (r *MemoryRepository) CountDoctorsBySpecialty(_ context.Context, specialtyID int64) (int, error) {
	doctors, err := r.ListDoctorsBySpecialty(context.Background(), specialtyID, 0, 0)
	return len(doctors), err
}

func (r *MemoryRepository) GetDoctorByID(_ context.Context, doctorID int64) (Doctor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	doc, ok := r.doctors[doctorID]
	if !ok {
		return Doctor{}, ErrNotFound
	}
	return doc, nil
}

func (r *MemoryRepository) ListAvailableDoctorSlots(_ context.Context, specialtyID, doctorID int64, limit, offset int) ([]DoctorSlot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []DoctorSlot
	for _, s := range r.doctorSlots {
		if s.SpecialtyID == specialtyID && s.DoctorID == doctorID && s.IsAvailable {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartAt.Before(out[j].StartAt) })
	return pageDoctorSlots(out, limit, offset), nil
}

func (r *MemoryRepository) CountAvailableDoctorSlots(_ context.Context, specialtyID, doctorID int64) (int, error) {
	slots, err := r.ListAvailableDoctorSlots(context.Background(), specialtyID, doctorID, 0, 0)
	return len(slots), err
}

func (r *MemoryRepository) GetDoctorSlotByID(_ context.Context, slotID int64) (DoctorSlot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	slot, ok := r.doctorSlots[slotID]
	if !ok {
		return DoctorSlot{}, ErrNotFound
	}
	return slot, nil
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

func (r *MemoryRepository) toClinicBookingViewLocked(b ClinicBooking) ClinicBookingView {
	slot := r.doctorSlots[b.DoctorSlotID]
	doc := r.doctors[b.DoctorID]
	spec := r.specialties[b.SpecialtyID]
	return ClinicBookingView{
		ID:            b.ID,
		SpecialtyName: spec.Name,
		DoctorName:    doc.FullName,
		StartAt:       slot.StartAt,
		Status:        b.Status,
		CreatedAt:     b.CreatedAt,
	}
}

func pageSpecialties(items []Specialty, limit, offset int) []Specialty {
	start, end := pageBounds(len(items), limit, offset)
	return append([]Specialty(nil), items[start:end]...)
}

func pageDoctors(items []Doctor, limit, offset int) []Doctor {
	start, end := pageBounds(len(items), limit, offset)
	return append([]Doctor(nil), items[start:end]...)
}

func pageDoctorSlots(items []DoctorSlot, limit, offset int) []DoctorSlot {
	start, end := pageBounds(len(items), limit, offset)
	return append([]DoctorSlot(nil), items[start:end]...)
}

func pageBounds(length, limit, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset >= length {
		return length, length
	}
	if limit <= 0 {
		limit = length
	}
	end := offset + limit
	if end > length {
		end = length
	}
	return offset, end
}
