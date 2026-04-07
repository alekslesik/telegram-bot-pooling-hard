package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
)

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

// DoctorSlotDayView represents a single doctor_slot during an admin view of a day.
// IsAvailable reflects the current "is_available" flag in doctor_slots.
// IsBooked reflects whether there is an existing confirmed clinic_bookings row.
type DoctorSlotDayView struct {
	ID          int64
	StartAt     time.Time
	IsAvailable bool
	IsBooked    bool
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

type CancelClinicBookingResult struct {
	Booking       ClinicBookingView
	RefundedCents int64
	BalanceAfter  int64
	RefundApplied bool
}

type WalletTransaction struct {
	ID               int64
	TelegramUserID   int64
	OperationID      string
	TxType           string
	AmountCents      int64
	BalanceBefore    int64
	BalanceAfter     int64
	RelatedBookingID *int64
	MetadataJSON     string
	CreatedAt        time.Time
}

type OutboxEvent struct {
	ID            int64
	DedupeKey     string
	EventType     string
	AggregateType string
	AggregateID   *int64
	PayloadJSON   string
	Status        string
	Attempts      int
	AvailableAt   time.Time
	LockedAt      *time.Time
	ProcessedAt   *time.Time
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UserDocument struct {
	ID             int64
	TelegramUserID int64
	FileID         string
	FileName       string
	MimeType       string
	FileSize       int
	CreatedAt      time.Time
}

type AdminAuditLog struct {
	ID          int64
	AdminUserID int64
	Action      string
	Details     string
	CreatedAt   time.Time
}

// UserProfile holds Level-3 account fields (balance, referrals, locale).
type UserProfile struct {
	TelegramUserID        int64
	BalanceCents          int64
	ReferralCode          string
	ReferredByTelegramID  *int64
	PreferredLang         string
	ReferralRewardGranted bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// PaidBookingResult is returned after an atomic paid clinic booking commit.
type PaidBookingResult struct {
	BookingID      int64
	SpecialtyName  string
	DoctorName     string
	SlotStart      time.Time
	BalanceAfter   int64
	BookingCreated time.Time
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
	ConfirmServiceBooking(ctx context.Context, booking Booking) (Booking, error)
	CreateClinicBooking(ctx context.Context, booking ClinicBooking) (ClinicBooking, error)
	MarkDoctorSlotUnavailable(ctx context.Context, slotID int64) error
	ListUserClinicBookings(ctx context.Context, userID int64, limit, offset int) ([]ClinicBookingView, error)
	CountUserClinicBookings(ctx context.Context, userID int64) (int, error)
	CancelClinicBooking(ctx context.Context, userID, bookingID int64) (CancelClinicBookingResult, error)
	SaveUserDocument(ctx context.Context, doc UserDocument) (UserDocument, error)
	ListRecentUserDocuments(ctx context.Context, userID int64, limit int) ([]UserDocument, error)

	IsAdmin(ctx context.Context, userID int64) (bool, error)
	ListAllSpecialties(ctx context.Context) ([]Specialty, error)
	ListAllDoctors(ctx context.Context) ([]Doctor, error)
	CreateSpecialty(ctx context.Context, name string, sortOrder int) (Specialty, error)
	CreateDoctor(ctx context.Context, fullName string) (Doctor, error)
	LinkDoctorToSpecialty(ctx context.Context, doctorID, specialtyID int64) error
	GenerateDoctorSlots(ctx context.Context, doctorID, specialtyID int64, date time.Time, startMinute, endMinute, stepMinutes int) (int, error)
	LogAdminAction(ctx context.Context, adminUserID int64, action, details string) error

	// Day tools (admin): close/open availability and view slot utilization.
	CloseDoctorDay(ctx context.Context, doctorID, specialtyID int64, date time.Time) (int, error)
	OpenDoctorDay(ctx context.Context, doctorID, specialtyID int64, date time.Time) (int, error)
	ListDoctorSlotsForDay(ctx context.Context, doctorID, specialtyID int64, date time.Time) ([]DoctorSlotDayView, error)

	GetConversationState(ctx context.Context, userID int64) (ConversationState, error)
	SaveConversationState(ctx context.Context, state ConversationState) error
	DeleteConversationState(ctx context.Context, userID int64) error

	// Level 3: profiles, analytics, paid booking.
	EnsureUserProfile(ctx context.Context, userID int64) (UserProfile, error)
	GetUserProfile(ctx context.Context, userID int64) (UserProfile, error)
	SetPreferredLang(ctx context.Context, userID int64, lang string) error
	ApplyReferralCodeIfNew(ctx context.Context, userID int64, code string) error
	GrantReferralRewardsOnRegistration(ctx context.Context, userID, refereeBonusCents, referrerBonusCents int64) error
	LogAnalyticsEvent(ctx context.Context, userID *int64, eventType, payloadJSON string) error
	CountAnalyticsByEventSince(ctx context.Context, since time.Time) (map[string]int64, error)
	ConfirmPaidClinicBooking(ctx context.Context, userID, feeCents, specialtyID, doctorID, slotID int64, operationID string) (PaidBookingResult, error)
	EnqueueOutboxEvent(ctx context.Context, event OutboxEvent) (OutboxEvent, error)
	ClaimDueOutboxEvents(ctx context.Context, limit int, now time.Time) ([]OutboxEvent, error)
	MarkOutboxEventDone(ctx context.Context, eventID int64) error
	MarkOutboxEventFailed(ctx context.Context, eventID int64, lastError string, nextAttemptAt time.Time) error
	MarkOutboxEventDead(ctx context.Context, eventID int64, lastError string) error
	CountOutboxByStatus(ctx context.Context) (map[string]int64, error)
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
	documents     map[int64]UserDocument
	nextBookingID int64
	nextServiceID int64
	nextSlotID    int64
	nextClinicID  int64
	nextDocID     int64

	admins       map[int64]struct{}
	adminLogs    []AdminAuditLog
	nextAdminLog int64

	userProfiles    map[int64]UserProfile
	analyticsEvents []memoryAnalyticsEvent
	nextAnalyticID  int64
	walletTx        map[int64]WalletTransaction
	walletTxByOp    map[string]int64
	nextWalletTxID  int64
	outboxEvents    map[int64]OutboxEvent
	nextOutboxID    int64
}

type memoryAnalyticsEvent struct {
	ID        int64
	UserID    *int64
	EventType string
	Payload   string
	CreatedAt time.Time
}

func NewMemoryRepository() *MemoryRepository {
	r := &MemoryRepository{
		services:        make(map[int64]Service),
		slots:           make(map[int64]Slot),
		bookings:        make(map[int64]Booking),
		states:          make(map[int64]ConversationState),
		clients:         make(map[int64]Client),
		specialties:     make(map[int64]Specialty),
		doctors:         make(map[int64]Doctor),
		doctorLinks:     make(map[int64]map[int64]struct{}),
		doctorSlots:     make(map[int64]DoctorSlot),
		clinicBooking:   make(map[int64]ClinicBooking),
		documents:       make(map[int64]UserDocument),
		nextBookingID:   1,
		nextServiceID:   1,
		nextSlotID:      1,
		nextClinicID:    1,
		nextDocID:       1,
		admins:          make(map[int64]struct{}),
		adminLogs:       []AdminAuditLog{},
		nextAdminLog:    1,
		userProfiles:    make(map[int64]UserProfile),
		analyticsEvents: []memoryAnalyticsEvent{},
		nextAnalyticID:  1,
		walletTx:        make(map[int64]WalletTransaction),
		walletTxByOp:    make(map[string]int64),
		nextWalletTxID:  1,
		outboxEvents:    make(map[int64]OutboxEvent),
		nextOutboxID:    1,
	}
	r.seed()
	return r
}

func randomReferralCode() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToLower(hex.EncodeToString(b[:])), nil
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

func (r *MemoryRepository) ConfirmServiceBooking(_ context.Context, booking Booking) (Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	slot, ok := r.slots[booking.SlotID]
	if !ok || !slot.IsAvailable {
		return Booking{}, ErrNotFound
	}
	slot.IsAvailable = false
	r.slots[booking.SlotID] = slot

	booking.ID = r.nextBookingID
	r.nextBookingID++
	if booking.CreatedAt.IsZero() {
		booking.CreatedAt = time.Now().UTC()
	}
	r.bookings[booking.ID] = booking
	return booking, nil
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
	now := time.Now().UTC()
	var out []ClinicBookingView
	for _, b := range r.clinicBooking {
		if b.TelegramUserID != userID {
			continue
		}
		item := r.toClinicBookingViewLocked(b)
		if item.Status != "confirmed" || item.StartAt.Before(now) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartAt.Before(out[j].StartAt) })
	start, end := pageBounds(len(out), limit, offset)
	return append([]ClinicBookingView(nil), out[start:end]...), nil
}

func (r *MemoryRepository) CountUserClinicBookings(_ context.Context, userID int64) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now().UTC()
	count := 0
	for _, b := range r.clinicBooking {
		if b.TelegramUserID != userID {
			continue
		}
		item := r.toClinicBookingViewLocked(b)
		if item.Status == "confirmed" && !item.StartAt.Before(now) {
			count++
		}
	}
	return count, nil
}

func (r *MemoryRepository) CancelClinicBooking(_ context.Context, userID, bookingID int64) (CancelClinicBookingResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.clinicBooking[bookingID]
	if !ok || b.TelegramUserID != userID {
		return CancelClinicBookingResult{}, ErrNotFound
	}
	if b.Status == "cancelled" {
		return CancelClinicBookingResult{
			Booking: r.toClinicBookingViewLocked(b),
		}, nil
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
	bookingIDCopy := bookingID
	_ = r.enqueueOutboxLocked(fmt.Sprintf("booking_cancelled:%d", bookingID), "booking_cancelled", "clinic_booking", &bookingIDCopy, fmt.Sprintf(`{"booking_id":%d,"user_id":%d}`, bookingID, userID), now)

	var refunded int64
	var balanceAfter int64
	for _, tx := range r.walletTx {
		if tx.TxType != "debit" || tx.RelatedBookingID == nil || *tx.RelatedBookingID != bookingID {
			continue
		}
		refundOp := "clinic_booking:refund:" + strconv.FormatInt(bookingID, 10)
		if _, exists := r.walletTxByOp[refundOp]; exists {
			break
		}
		profile, ok := r.userProfiles[userID]
		if !ok {
			break
		}
		refunded = -tx.AmountCents
		if refunded <= 0 {
			break
		}
		before := profile.BalanceCents
		profile.BalanceCents += refunded
		profile.UpdatedAt = now
		balanceAfter = profile.BalanceCents
		r.userProfiles[userID] = profile
		wtx := WalletTransaction{
			ID:               r.nextWalletTxID,
			TelegramUserID:   userID,
			OperationID:      refundOp,
			TxType:           "refund",
			AmountCents:      refunded,
			BalanceBefore:    before,
			BalanceAfter:     balanceAfter,
			RelatedBookingID: &bookingID,
			MetadataJSON:     "{}",
			CreatedAt:        now,
		}
		r.walletTx[wtx.ID] = wtx
		r.walletTxByOp[refundOp] = wtx.ID
		r.nextWalletTxID++
		_ = r.enqueueOutboxLocked(fmt.Sprintf("booking_refunded:%d", bookingID), "booking_refunded", "clinic_booking", &bookingIDCopy, fmt.Sprintf(`{"booking_id":%d,"user_id":%d,"refunded_cents":%d}`, bookingID, userID, refunded), now)
		break
	}
	return CancelClinicBookingResult{
		Booking:       r.toClinicBookingViewLocked(b),
		RefundedCents: refunded,
		BalanceAfter:  balanceAfter,
		RefundApplied: refunded > 0,
	}, nil
}

func (r *MemoryRepository) SaveUserDocument(_ context.Context, doc UserDocument) (UserDocument, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	doc.ID = r.nextDocID
	r.nextDocID++
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	r.documents[doc.ID] = doc
	return doc, nil
}

func (r *MemoryRepository) ListRecentUserDocuments(_ context.Context, userID int64, limit int) ([]UserDocument, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []UserDocument
	for _, d := range r.documents {
		if d.TelegramUserID == userID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return append([]UserDocument(nil), out...), nil
}

func (r *MemoryRepository) IsAdmin(_ context.Context, userID int64) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.admins[userID]
	return ok, nil
}

func (r *MemoryRepository) ListAllSpecialties(_ context.Context) ([]Specialty, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Specialty, 0, len(r.specialties))
	for _, s := range r.specialties {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder == out[j].SortOrder {
			return out[i].ID < out[j].ID
		}
		return out[i].SortOrder < out[j].SortOrder
	})
	return out, nil
}

func (r *MemoryRepository) ListAllDoctors(_ context.Context) ([]Doctor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Doctor, 0, len(r.doctors))
	for _, d := range r.doctors {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *MemoryRepository) CreateSpecialty(_ context.Context, name string, sortOrder int) (Specialty, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name = strings.TrimSpace(name)
	for _, s := range r.specialties {
		if strings.EqualFold(s.Name, name) {
			// update instead of creating duplicates
			s.SortOrder = sortOrder
			s.IsActive = true
			r.specialties[s.ID] = s
			return s, nil
		}
	}
	var maxID int64
	for id := range r.specialties {
		if id > maxID {
			maxID = id
		}
	}
	s := Specialty{ID: maxID + 1, Name: name, SortOrder: sortOrder, IsActive: true}
	r.specialties[s.ID] = s
	return s, nil
}

func (r *MemoryRepository) CreateDoctor(_ context.Context, fullName string) (Doctor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fullName = strings.TrimSpace(fullName)
	for _, d := range r.doctors {
		if strings.EqualFold(d.FullName, fullName) {
			d.IsActive = true
			r.doctors[d.ID] = d
			return d, nil
		}
	}
	var maxID int64
	for id := range r.doctors {
		if id > maxID {
			maxID = id
		}
	}
	d := Doctor{ID: maxID + 1, FullName: fullName, IsActive: true}
	r.doctors[d.ID] = d
	return d, nil
}

func (r *MemoryRepository) LinkDoctorToSpecialty(_ context.Context, doctorID, specialtyID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.doctors[doctorID]; !ok {
		return ErrNotFound
	}
	if _, ok := r.specialties[specialtyID]; !ok {
		return ErrNotFound
	}
	if _, ok := r.doctorLinks[doctorID]; !ok {
		r.doctorLinks[doctorID] = make(map[int64]struct{})
	}
	r.doctorLinks[doctorID][specialtyID] = struct{}{}
	return nil
}

func (r *MemoryRepository) GenerateDoctorSlots(_ context.Context, doctorID, specialtyID int64, date time.Time, startMinute, endMinute, stepMinutes int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if stepMinutes <= 0 || endMinute <= startMinute {
		return 0, nil
	}
	base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	var inserted int

	var maxID int64
	for id := range r.doctorSlots {
		if id > maxID {
			maxID = id
		}
	}

	for m := startMinute; m < endMinute; m += stepMinutes {
		at := base.Add(time.Duration(m) * time.Minute)
		already := false
		for _, s := range r.doctorSlots {
			if s.DoctorID == doctorID && s.SpecialtyID == specialtyID && s.StartAt.Equal(at) {
				already = true
				break
			}
		}
		if already {
			continue
		}
		maxID++
		r.doctorSlots[maxID] = DoctorSlot{
			ID:          maxID,
			DoctorID:    doctorID,
			SpecialtyID: specialtyID,
			StartAt:     at,
			IsAvailable: true,
		}
		inserted++
	}

	return inserted, nil
}

func (r *MemoryRepository) CloseDoctorDay(_ context.Context, doctorID, specialtyID int64, date time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	updated := 0

	for id, slot := range r.doctorSlots {
		if slot.DoctorID != doctorID || slot.SpecialtyID != specialtyID {
			continue
		}
		slotDay := time.Date(slot.StartAt.Year(), slot.StartAt.Month(), slot.StartAt.Day(), 0, 0, 0, 0, time.UTC)
		if slotDay.Equal(date) {
			if slot.IsAvailable {
				updated++
			}
			slot.IsAvailable = false
			r.doctorSlots[id] = slot
		}
	}

	return updated, nil
}

func (r *MemoryRepository) OpenDoctorDay(_ context.Context, doctorID, specialtyID int64, date time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	updated := 0

	for id, slot := range r.doctorSlots {
		if slot.DoctorID != doctorID || slot.SpecialtyID != specialtyID {
			continue
		}
		slotDay := time.Date(slot.StartAt.Year(), slot.StartAt.Month(), slot.StartAt.Day(), 0, 0, 0, 0, time.UTC)
		if !slotDay.Equal(date) {
			continue
		}

		isBooked := false
		for _, b := range r.clinicBooking {
			if b.DoctorSlotID == id && b.Status == "confirmed" {
				isBooked = true
				break
			}
		}
		if isBooked {
			continue
		}

		if !slot.IsAvailable {
			updated++
		}
		slot.IsAvailable = true
		r.doctorSlots[id] = slot
	}

	return updated, nil
}

func (r *MemoryRepository) ListDoctorSlotsForDay(_ context.Context, doctorID, specialtyID int64, date time.Time) ([]DoctorSlotDayView, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	var out []DoctorSlotDayView
	for _, slot := range r.doctorSlots {
		if slot.DoctorID != doctorID || slot.SpecialtyID != specialtyID {
			continue
		}
		slotDay := time.Date(slot.StartAt.Year(), slot.StartAt.Month(), slot.StartAt.Day(), 0, 0, 0, 0, time.UTC)
		if !slotDay.Equal(date) {
			continue
		}

		isBooked := false
		for _, b := range r.clinicBooking {
			if b.DoctorSlotID == slot.ID && b.Status == "confirmed" {
				isBooked = true
				break
			}
		}

		out = append(out, DoctorSlotDayView{
			ID:          slot.ID,
			StartAt:     slot.StartAt,
			IsAvailable: slot.IsAvailable,
			IsBooked:    isBooked,
		})
	}

	// Stable ordering by start time.
	sort.Slice(out, func(i, j int) bool { return out[i].StartAt.Before(out[j].StartAt) })
	return out, nil
}

func (r *MemoryRepository) LogAdminAction(_ context.Context, adminUserID int64, action, details string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adminLogs = append(r.adminLogs, AdminAuditLog{
		ID:          r.nextAdminLog,
		AdminUserID: adminUserID,
		Action:      action,
		Details:     details,
		CreatedAt:   time.Now().UTC(),
	})
	r.nextAdminLog++
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

func (r *MemoryRepository) referralCodeTakenLocked(code string) bool {
	for _, p := range r.userProfiles {
		if p.ReferralCode == code {
			return true
		}
	}
	return false
}

func (r *MemoryRepository) EnsureUserProfile(_ context.Context, userID int64) (UserProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.userProfiles[userID]; ok {
		return p, nil
	}
	for {
		code, err := randomReferralCode()
		if err != nil {
			return UserProfile{}, err
		}
		if r.referralCodeTakenLocked(code) {
			continue
		}
		now := time.Now().UTC()
		p := UserProfile{
			TelegramUserID:        userID,
			BalanceCents:          500,
			ReferralCode:          code,
			PreferredLang:         "ru",
			ReferralRewardGranted: false,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		r.userProfiles[userID] = p
		return p, nil
	}
}

func (r *MemoryRepository) GetUserProfile(_ context.Context, userID int64) (UserProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.userProfiles[userID]
	if !ok {
		return UserProfile{}, ErrNotFound
	}
	return p, nil
}

func (r *MemoryRepository) SetPreferredLang(_ context.Context, userID int64, lang string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.userProfiles[userID]
	if !ok {
		return ErrNotFound
	}
	lang = strings.TrimSpace(strings.ToLower(lang))
	if lang != "en" && lang != "ru" {
		lang = "ru"
	}
	p.PreferredLang = lang
	p.UpdatedAt = time.Now().UTC()
	r.userProfiles[userID] = p
	return nil
}

func (r *MemoryRepository) ApplyReferralCodeIfNew(_ context.Context, userID int64, code string) error {
	code = strings.TrimSpace(strings.ToLower(code))
	if code == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.userProfiles[userID]
	if !ok {
		return ErrNotFound
	}
	if p.ReferredByTelegramID != nil {
		return nil
	}
	var refID int64
	found := false
	for id, prof := range r.userProfiles {
		if prof.ReferralCode == code && id != userID {
			refID = id
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	p.ReferredByTelegramID = int64Ptr(refID)
	p.UpdatedAt = time.Now().UTC()
	r.userProfiles[userID] = p
	return nil
}

func int64Ptr(v int64) *int64 {
	return &v
}

func (r *MemoryRepository) GrantReferralRewardsOnRegistration(_ context.Context, userID, refereeBonusCents, referrerBonusCents int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.userProfiles[userID]
	if !ok {
		return ErrNotFound
	}
	if p.ReferralRewardGranted || p.ReferredByTelegramID == nil {
		return nil
	}
	refID := *p.ReferredByTelegramID
	ref, ok := r.userProfiles[refID]
	if !ok {
		return nil
	}
	p.BalanceCents += refereeBonusCents
	p.ReferralRewardGranted = true
	p.UpdatedAt = time.Now().UTC()
	r.userProfiles[userID] = p

	ref.BalanceCents += referrerBonusCents
	ref.UpdatedAt = time.Now().UTC()
	r.userProfiles[refID] = ref
	return nil
}

func (r *MemoryRepository) LogAnalyticsEvent(_ context.Context, userID *int64, eventType, payloadJSON string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(payloadJSON) == "" {
		payloadJSON = "{}"
	}
	r.analyticsEvents = append(r.analyticsEvents, memoryAnalyticsEvent{
		ID:        r.nextAnalyticID,
		UserID:    userID,
		EventType: eventType,
		Payload:   payloadJSON,
		CreatedAt: time.Now().UTC(),
	})
	r.nextAnalyticID++
	return nil
}

func (r *MemoryRepository) CountAnalyticsByEventSince(_ context.Context, since time.Time) (map[string]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]int64)
	for _, e := range r.analyticsEvents {
		if e.CreatedAt.Before(since) {
			continue
		}
		out[e.EventType]++
	}
	return out, nil
}

func (r *MemoryRepository) ConfirmPaidClinicBooking(_ context.Context, userID, feeCents, specialtyID, doctorID, slotID int64, operationID string) (PaidBookingResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if operationID != "" {
		if txID, ok := r.walletTxByOp[operationID]; ok {
			tx := r.walletTx[txID]
			if tx.TxType == "debit" && tx.RelatedBookingID != nil {
				if booking, ok := r.clinicBooking[*tx.RelatedBookingID]; ok {
					spec := r.specialties[booking.SpecialtyID]
					doc := r.doctors[booking.DoctorID]
					slot := r.doctorSlots[booking.DoctorSlotID]
					return PaidBookingResult{
						BookingID:      booking.ID,
						SpecialtyName:  spec.Name,
						DoctorName:     doc.FullName,
						SlotStart:      slot.StartAt,
						BalanceAfter:   tx.BalanceAfter,
						BookingCreated: booking.CreatedAt,
					}, nil
				}
			}
		}
	}

	p, ok := r.userProfiles[userID]
	if !ok {
		for {
			code, err := randomReferralCode()
			if err != nil {
				return PaidBookingResult{}, err
			}
			if r.referralCodeTakenLocked(code) {
				continue
			}
			now := time.Now().UTC()
			p = UserProfile{
				TelegramUserID:        userID,
				BalanceCents:          500,
				ReferralCode:          code,
				PreferredLang:         "ru",
				ReferralRewardGranted: false,
				CreatedAt:             now,
				UpdatedAt:             now,
			}
			r.userProfiles[userID] = p
			break
		}
	}

	if p.BalanceCents < feeCents {
		return PaidBookingResult{}, ErrInsufficientFunds
	}

	slot, ok := r.doctorSlots[slotID]
	if !ok || !slot.IsAvailable || slot.DoctorID != doctorID || slot.SpecialtyID != specialtyID {
		return PaidBookingResult{}, ErrNotFound
	}

	before := p.BalanceCents
	p.BalanceCents -= feeCents
	p.UpdatedAt = time.Now().UTC()
	r.userProfiles[userID] = p

	slot.IsAvailable = false
	r.doctorSlots[slotID] = slot

	booking := ClinicBooking{
		ID:             r.nextClinicID,
		TelegramUserID: userID,
		SpecialtyID:    specialtyID,
		DoctorID:       doctorID,
		DoctorSlotID:   slotID,
		Status:         "confirmed",
		CreatedAt:      time.Now().UTC(),
	}
	r.nextClinicID++
	r.clinicBooking[booking.ID] = booking

	if operationID == "" {
		operationID = "clinic_booking:confirm:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(slotID, 10)
	}
	wtx := WalletTransaction{
		ID:               r.nextWalletTxID,
		TelegramUserID:   userID,
		OperationID:      operationID,
		TxType:           "debit",
		AmountCents:      -feeCents,
		BalanceBefore:    before,
		BalanceAfter:     p.BalanceCents,
		RelatedBookingID: &booking.ID,
		MetadataJSON:     "{}",
		CreatedAt:        booking.CreatedAt,
	}
	r.walletTx[wtx.ID] = wtx
	r.walletTxByOp[operationID] = wtx.ID
	r.nextWalletTxID++
	bookingIDCopy := booking.ID
	_ = r.enqueueOutboxLocked(fmt.Sprintf("booking_confirmed:%d", booking.ID), "booking_confirmed", "clinic_booking", &bookingIDCopy, fmt.Sprintf(`{"booking_id":%d,"user_id":%d,"specialty_id":%d,"doctor_id":%d,"slot_id":%d}`, booking.ID, userID, specialtyID, doctorID, slotID), booking.CreatedAt)

	spec := r.specialties[specialtyID]
	doc := r.doctors[doctorID]

	return PaidBookingResult{
		BookingID:      booking.ID,
		SpecialtyName:  spec.Name,
		DoctorName:     doc.FullName,
		SlotStart:      slot.StartAt,
		BalanceAfter:   p.BalanceCents,
		BookingCreated: booking.CreatedAt,
	}, nil
}

func (r *MemoryRepository) EnqueueOutboxEvent(_ context.Context, event OutboxEvent) (OutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(event.DedupeKey) != "" {
		for _, existing := range r.outboxEvents {
			if existing.DedupeKey == event.DedupeKey {
				return existing, nil
			}
		}
	}
	return r.enqueueOutboxLocked(event.DedupeKey, event.EventType, event.AggregateType, event.AggregateID, event.PayloadJSON, event.AvailableAt), nil
}

func (r *MemoryRepository) enqueueOutboxLocked(dedupeKey, eventType, aggregateType string, aggregateID *int64, payload string, availableAt time.Time) OutboxEvent {
	now := time.Now().UTC()
	if availableAt.IsZero() {
		availableAt = now
	}
	if strings.TrimSpace(payload) == "" {
		payload = "{}"
	}
	id := r.nextOutboxID
	r.nextOutboxID++
	ev := OutboxEvent{
		ID:            id,
		DedupeKey:     strings.TrimSpace(dedupeKey),
		EventType:     strings.TrimSpace(eventType),
		AggregateType: strings.TrimSpace(aggregateType),
		AggregateID:   aggregateID,
		PayloadJSON:   payload,
		Status:        "pending",
		Attempts:      0,
		AvailableAt:   availableAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	r.outboxEvents[id] = ev
	return ev
}

func (r *MemoryRepository) ClaimDueOutboxEvents(_ context.Context, limit int, now time.Time) ([]OutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 {
		limit = 10
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ids := make([]int64, 0, len(r.outboxEvents))
	for id, ev := range r.outboxEvents {
		if ev.Status == "pending" && !ev.AvailableAt.After(now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]OutboxEvent, 0, len(ids))
	for _, id := range ids {
		ev := r.outboxEvents[id]
		lockTime := now
		ev.Status = "processing"
		ev.Attempts++
		ev.LockedAt = &lockTime
		ev.UpdatedAt = now
		r.outboxEvents[id] = ev
		out = append(out, ev)
	}
	return out, nil
}

func (r *MemoryRepository) MarkOutboxEventDone(_ context.Context, eventID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev, ok := r.outboxEvents[eventID]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	ev.Status = "done"
	ev.LockedAt = nil
	ev.ProcessedAt = &now
	ev.UpdatedAt = now
	r.outboxEvents[eventID] = ev
	return nil
}

func (r *MemoryRepository) MarkOutboxEventFailed(_ context.Context, eventID int64, lastError string, nextAttemptAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev, ok := r.outboxEvents[eventID]
	if !ok {
		return ErrNotFound
	}
	if nextAttemptAt.IsZero() {
		nextAttemptAt = time.Now().UTC().Add(30 * time.Second)
	}
	ev.Status = "pending"
	ev.LockedAt = nil
	ev.LastError = strings.TrimSpace(lastError)
	ev.AvailableAt = nextAttemptAt
	ev.UpdatedAt = time.Now().UTC()
	r.outboxEvents[eventID] = ev
	return nil
}

func (r *MemoryRepository) MarkOutboxEventDead(_ context.Context, eventID int64, lastError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev, ok := r.outboxEvents[eventID]
	if !ok {
		return ErrNotFound
	}
	ev.Status = "failed"
	ev.LockedAt = nil
	ev.LastError = strings.TrimSpace(lastError)
	now := time.Now().UTC()
	ev.ProcessedAt = &now
	ev.UpdatedAt = now
	r.outboxEvents[eventID] = ev
	return nil
}

func (r *MemoryRepository) CountOutboxByStatus(_ context.Context) (map[string]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]int64{
		"pending":    0,
		"processing": 0,
		"done":       0,
		"failed":     0,
	}
	for _, ev := range r.outboxEvents {
		out[ev.Status]++
	}
	return out, nil
}
