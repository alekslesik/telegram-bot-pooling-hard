package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/i18n"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

const (
	StateWaitingName              = "waiting_name"
	StateWaitingPhone             = "waiting_phone"
	StateWaitingService           = "waiting_service"
	StateWaitingSlot              = "waiting_slot"
	StateWaitingConfirm           = "waiting_confirm"
	StateWaitingDocument          = "waiting_document"
	StateAdminAddSpecialty        = "admin_add_specialty"
	StateAdminAddDoctor           = "admin_add_doctor"
	StateAdminLinkDoctorSpecialty = "admin_link_doctor_specialty"
	StateAdminGenerateSlots       = "admin_generate_slots"
	StateAdminCloseDay            = "admin_close_day"
	StateAdminOpenDay             = "admin_open_day"
	StateAdminDaySlots            = "admin_day_slots"
)

type statePayload struct {
	FullName  string `json:"full_name"`
	Phone     string `json:"phone"`
	ServiceID int64  `json:"service_id"`
	SlotID    int64  `json:"slot_id"`
}

var phoneCleaner = regexp.MustCompile(`[^0-9+]`)

type BookingService struct {
	repo  repository.BookingRepository
	cache SpecialtyPageCache
}

type AdminCapabilities struct {
	CanOpenPanel      bool
	CanManageCatalog  bool
	CanManageDaySlots bool
	CanViewAnalytics  bool
}

func NewBookingService(repo repository.BookingRepository, cache SpecialtyPageCache) *BookingService {
	return &BookingService{repo: repo, cache: cache}
}

func canManageCatalog(role repository.AdminRole) bool {
	return role == repository.AdminRoleOwner || role == repository.AdminRoleAdmin
}

func canManageDaySlots(role repository.AdminRole) bool {
	return role == repository.AdminRoleOwner || role == repository.AdminRoleAdmin || role == repository.AdminRoleOperator
}

func canViewAnalytics(role repository.AdminRole) bool {
	return role == repository.AdminRoleOwner || role == repository.AdminRoleAdmin
}

func (s *BookingService) getAdminRole(ctx context.Context, userID int64) (repository.AdminRole, error) {
	role, err := s.repo.GetAdminRole(ctx, userID)
	if err == repository.ErrNotFound {
		return "", nil
	}
	return role, err
}

func (s *BookingService) AdminCapabilities(ctx context.Context, userID int64) (AdminCapabilities, error) {
	role, err := s.getAdminRole(ctx, userID)
	if err != nil {
		return AdminCapabilities{}, err
	}
	if role == "" {
		return AdminCapabilities{}, nil
	}
	return AdminCapabilities{
		CanOpenPanel:      true,
		CanManageCatalog:  canManageCatalog(role),
		CanManageDaySlots: canManageDaySlots(role),
		CanViewAnalytics:  canViewAnalytics(role),
	}, nil
}

func (s *BookingService) denyAdminAndReset(ctx context.Context, userID int64) (bool, string, error) {
	_ = s.repo.DeleteConversationState(ctx, userID)
	return true, "Нет доступа к админ-панели.", nil
}

func (s *BookingService) Start(ctx context.Context, userID int64) (string, error) {
	client, err := s.repo.GetClientByUserID(ctx, userID)
	switch {
	case err == nil:
		if strings.TrimSpace(client.FullName) != "" && strings.TrimSpace(client.Phone) != "" {
			return "Выберите направление и врача в меню ниже.", nil
		}
	case err != repository.ErrNotFound:
		return "", err
	}
	if err := s.saveState(ctx, userID, StateWaitingName, statePayload{}); err != nil {
		return "", err
	}
	return "Перед записью введите, пожалуйста, ваше ФИО.", nil
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
	case StateWaitingName:
		return s.handleNameInput(ctx, userID, payload, text)
	case StateWaitingPhone:
		return s.handlePhoneInput(ctx, userID, payload, text)
	case StateWaitingService:
		return s.handleServiceSelection(ctx, userID, payload, text)
	case StateWaitingSlot:
		return s.handleSlotSelection(ctx, userID, payload, text)
	case StateWaitingConfirm:
		return s.handleConfirmation(ctx, userID, payload, text)
	case StateWaitingDocument:
		return true, "Ожидаю документ. Отправьте файл или фото.", nil
	case StateAdminAddSpecialty:
		return s.handleAdminAddSpecialty(ctx, userID, text)
	case StateAdminAddDoctor:
		return s.handleAdminAddDoctor(ctx, userID, text)
	case StateAdminLinkDoctorSpecialty:
		return s.handleAdminLinkDoctorSpecialty(ctx, userID, text)
	case StateAdminGenerateSlots:
		return s.handleAdminGenerateSlots(ctx, userID, text)
	case StateAdminCloseDay:
		return s.handleAdminCloseDay(ctx, userID, text)
	case StateAdminOpenDay:
		return s.handleAdminOpenDay(ctx, userID, text)
	case StateAdminDaySlots:
		return s.handleAdminDaySlots(ctx, userID, text)
	default:
		return false, "", nil
	}
}

func (s *BookingService) StartDocumentUpload(ctx context.Context, userID int64) (string, error) {
	if err := s.saveState(ctx, userID, StateWaitingDocument, statePayload{}); err != nil {
		return "", err
	}
	return "Отправьте документ одним сообщением (файл или фото).", nil
}

func (s *BookingService) StartAdmin(ctx context.Context, userID int64) (bool, string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return false, "", err
	}
	if !caps.CanOpenPanel {
		return false, "Нет доступа к админ-панели.", nil
	}
	return true, "Админ-панель: выберите действие.", nil
}

func (s *BookingService) StartAdminAddSpecialty(ctx context.Context, userID int64) (string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return "", err
	}
	if !caps.CanManageCatalog {
		return "Нет доступа к админ-панели.", nil
	}
	if err := s.saveState(ctx, userID, StateAdminAddSpecialty, statePayload{}); err != nil {
		return "", err
	}
	return "Введите специализацию в формате: `Название|Порядок`.\nПример: Гастроэнтеролог|5", nil
}

func (s *BookingService) StartAdminAddDoctor(ctx context.Context, userID int64) (string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return "", err
	}
	if !caps.CanManageCatalog {
		return "Нет доступа к админ-панели.", nil
	}
	if err := s.saveState(ctx, userID, StateAdminAddDoctor, statePayload{}); err != nil {
		return "", err
	}
	return "Введите ФИО врача.\nПример: Волницкий Иван Васильевич", nil
}

func (s *BookingService) StartAdminLinkDoctorSpecialty(ctx context.Context, userID int64) (string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return "", err
	}
	if !caps.CanManageCatalog {
		return "Нет доступа к админ-панели.", nil
	}
	if err := s.saveState(ctx, userID, StateAdminLinkDoctorSpecialty, statePayload{}); err != nil {
		return "", err
	}
	dicts, err := s.adminDictionaries(ctx)
	if err != nil {
		return "", err
	}
	return dicts + "\n\nВведите связку: `doctor_id|specialty_id`. Пример: 2|5", nil
}

func (s *BookingService) StartAdminGenerateSlots(ctx context.Context, userID int64) (string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return "", err
	}
	if !caps.CanManageCatalog {
		return "Нет доступа к админ-панели.", nil
	}
	if err := s.saveState(ctx, userID, StateAdminGenerateSlots, statePayload{}); err != nil {
		return "", err
	}
	dicts, err := s.adminDictionaries(ctx)
	if err != nil {
		return "", err
	}
	return dicts + "\n\nВведите параметры: `doctor_id|specialty_id|YYYY-MM-DD|09:00|18:00|30`.\nШаг задайте в минутах (например 30).", nil
}

func (s *BookingService) StartAdminCloseDay(ctx context.Context, userID int64) (string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return "", err
	}
	if !caps.CanManageDaySlots {
		return "Нет доступа к админ-панели.", nil
	}
	if err := s.saveState(ctx, userID, StateAdminCloseDay, statePayload{}); err != nil {
		return "", err
	}
	return "Закрыть день (сделать слоты недоступными).\nВведите: doctor_id|specialty_id|YYYY-MM-DD.\nПример: 2|5|2026-03-30", nil
}

func (s *BookingService) StartAdminOpenDay(ctx context.Context, userID int64) (string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return "", err
	}
	if !caps.CanManageDaySlots {
		return "Нет доступа к админ-панели.", nil
	}
	if err := s.saveState(ctx, userID, StateAdminOpenDay, statePayload{}); err != nil {
		return "", err
	}
	return "Открыть день (включить слоты, если они не заняты).\nВведите: doctor_id|specialty_id|YYYY-MM-DD.\nПример: 2|5|2026-03-30", nil
}

func (s *BookingService) StartAdminDaySlots(ctx context.Context, userID int64) (string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return "", err
	}
	if !caps.CanManageDaySlots {
		return "Нет доступа к админ-панели.", nil
	}
	if err := s.saveState(ctx, userID, StateAdminDaySlots, statePayload{}); err != nil {
		return "", err
	}
	return "Показать слоты на день и загрузку.\nВведите: doctor_id|specialty_id|YYYY-MM-DD.\nПример: 2|5|2026-03-30", nil
}

func (s *BookingService) handleAdminAddSpecialty(ctx context.Context, userID int64, text string) (bool, string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return true, "", err
	}
	if !caps.CanManageCatalog {
		return s.denyAdminAndReset(ctx, userID)
	}
	parts := strings.Split(strings.TrimSpace(text), "|")
	if len(parts) != 2 {
		return true, "Неверный формат. Используйте: Название|Порядок", nil
	}
	name := strings.TrimSpace(parts[0])
	order, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return true, "Порядок должен быть числом.", nil
	}
	item, err := s.repo.CreateSpecialty(ctx, name, order)
	if err != nil {
		return true, "", err
	}
	_ = s.repo.LogAdminAction(ctx, userID, "create_specialty", fmt.Sprintf("id=%d name=%s sort=%d", item.ID, item.Name, item.SortOrder))
	_ = s.repo.DeleteConversationState(ctx, userID)
	return true, fmt.Sprintf("Специализация сохранена: ID %d, %s", item.ID, item.Name), nil
}

func (s *BookingService) handleAdminAddDoctor(ctx context.Context, userID int64, text string) (bool, string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return true, "", err
	}
	if !caps.CanManageCatalog {
		return s.denyAdminAndReset(ctx, userID)
	}
	name := strings.TrimSpace(text)
	if len(name) < 3 {
		return true, "Введите корректное ФИО врача.", nil
	}
	item, err := s.repo.CreateDoctor(ctx, name)
	if err != nil {
		return true, "", err
	}
	_ = s.repo.LogAdminAction(ctx, userID, "create_doctor", fmt.Sprintf("id=%d name=%s", item.ID, item.FullName))
	_ = s.repo.DeleteConversationState(ctx, userID)
	return true, fmt.Sprintf("Врач сохранен: ID %d, %s", item.ID, item.FullName), nil
}

func (s *BookingService) handleAdminLinkDoctorSpecialty(ctx context.Context, userID int64, text string) (bool, string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return true, "", err
	}
	if !caps.CanManageCatalog {
		return s.denyAdminAndReset(ctx, userID)
	}
	parts := strings.Split(strings.TrimSpace(text), "|")
	if len(parts) != 2 {
		return true, "Неверный формат. Используйте: doctor_id|specialty_id", nil
	}
	doctorID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return true, "doctor_id должен быть числом.", nil
	}
	specialtyID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return true, "specialty_id должен быть числом.", nil
	}
	if err := s.repo.LinkDoctorToSpecialty(ctx, doctorID, specialtyID); err != nil {
		if err == repository.ErrNotFound {
			return true, "doctor_id или specialty_id не найдены.", nil
		}
		return true, "", err
	}
	_ = s.repo.LogAdminAction(ctx, userID, "link_doctor_specialty", fmt.Sprintf("doctor_id=%d specialty_id=%d", doctorID, specialtyID))
	_ = s.repo.DeleteConversationState(ctx, userID)
	return true, "Связка врача и специализации сохранена.", nil
}

func (s *BookingService) handleAdminGenerateSlots(ctx context.Context, userID int64, text string) (bool, string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return true, "", err
	}
	if !caps.CanManageCatalog {
		return s.denyAdminAndReset(ctx, userID)
	}
	parts := strings.Split(strings.TrimSpace(text), "|")
	if len(parts) != 6 {
		return true, "Неверный формат. Используйте: doctor_id|specialty_id|YYYY-MM-DD|09:00|18:00|30", nil
	}
	doctorID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return true, "doctor_id должен быть числом.", nil
	}
	specialtyID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return true, "specialty_id должен быть числом.", nil
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(parts[2]))
	if err != nil {
		return true, "Дата должна быть в формате YYYY-MM-DD.", nil
	}
	startMinute, err := parseClockToMinute(parts[3])
	if err != nil {
		return true, "start_time должен быть в формате HH:MM.", nil
	}
	endMinute, err := parseClockToMinute(parts[4])
	if err != nil {
		return true, "end_time должен быть в формате HH:MM.", nil
	}
	step, err := strconv.Atoi(strings.TrimSpace(parts[5]))
	if err != nil || step <= 0 {
		return true, "step_minutes должен быть положительным числом.", nil
	}
	if endMinute <= startMinute {
		return true, "end_time должен быть позже start_time.", nil
	}

	inserted, err := s.repo.GenerateDoctorSlots(ctx, doctorID, specialtyID, date, startMinute, endMinute, step)
	if err != nil {
		if err == repository.ErrNotFound {
			return true, "doctor_id или specialty_id не найдены.", nil
		}
		return true, "", err
	}
	_ = s.repo.LogAdminAction(ctx, userID, "generate_slots", fmt.Sprintf("doctor_id=%d specialty_id=%d date=%s inserted=%d", doctorID, specialtyID, date.Format("2006-01-02"), inserted))
	_ = s.repo.DeleteConversationState(ctx, userID)
	return true, fmt.Sprintf("Готово. Добавлено слотов: %d", inserted), nil
}

func parseAdminDayInput(raw string) (doctorID, specialtyID int64, date time.Time, err error) {
	parts := strings.Split(strings.TrimSpace(raw), "|")
	if len(parts) != 3 {
		return 0, 0, time.Time{}, fmt.Errorf("invalid format")
	}
	doctorID, err = strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	specialtyID, err = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	date, err = time.Parse("2006-01-02", strings.TrimSpace(parts[2]))
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	return doctorID, specialtyID, date, nil
}

func (s *BookingService) handleAdminCloseDay(ctx context.Context, userID int64, text string) (bool, string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return true, "", err
	}
	if !caps.CanManageDaySlots {
		return s.denyAdminAndReset(ctx, userID)
	}
	doctorID, specialtyID, date, err := parseAdminDayInput(text)
	if err != nil {
		return true, "Неверный формат. Используйте: doctor_id|specialty_id|YYYY-MM-DD", nil
	}
	updated, err := s.repo.CloseDoctorDay(ctx, doctorID, specialtyID, date)
	if err != nil {
		return true, "", err
	}
	_ = s.repo.LogAdminAction(ctx, userID, "close_doctor_day", fmt.Sprintf("doctor_id=%d specialty_id=%d date=%s updated=%d", doctorID, specialtyID, date.Format("2006-01-02"), updated))
	_ = s.repo.DeleteConversationState(ctx, userID)
	return true, fmt.Sprintf("День закрыт. Изменено слотов: %d", updated), nil
}

func (s *BookingService) handleAdminOpenDay(ctx context.Context, userID int64, text string) (bool, string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return true, "", err
	}
	if !caps.CanManageDaySlots {
		return s.denyAdminAndReset(ctx, userID)
	}
	doctorID, specialtyID, date, err := parseAdminDayInput(text)
	if err != nil {
		return true, "Неверный формат. Используйте: doctor_id|specialty_id|YYYY-MM-DD", nil
	}
	updated, err := s.repo.OpenDoctorDay(ctx, doctorID, specialtyID, date)
	if err != nil {
		return true, "", err
	}
	_ = s.repo.LogAdminAction(ctx, userID, "open_doctor_day", fmt.Sprintf("doctor_id=%d specialty_id=%d date=%s updated=%d", doctorID, specialtyID, date.Format("2006-01-02"), updated))
	_ = s.repo.DeleteConversationState(ctx, userID)
	return true, fmt.Sprintf("День открыт. Включено слотов: %d", updated), nil
}

func (s *BookingService) handleAdminDaySlots(ctx context.Context, userID int64, text string) (bool, string, error) {
	caps, err := s.AdminCapabilities(ctx, userID)
	if err != nil {
		return true, "", err
	}
	if !caps.CanManageDaySlots {
		return s.denyAdminAndReset(ctx, userID)
	}
	doctorID, specialtyID, date, err := parseAdminDayInput(text)
	if err != nil {
		return true, "Неверный формат. Используйте: doctor_id|specialty_id|YYYY-MM-DD", nil
	}

	slots, err := s.repo.ListDoctorSlotsForDay(ctx, doctorID, specialtyID, date)
	if err != nil {
		return true, "", err
	}

	var free, closed, busy int
	var b strings.Builder
	dateStr := date.Format("2006-01-02")

	b.WriteString(fmt.Sprintf("Слоты на %s\nВрач ID: %d | Специализация ID: %d\n", dateStr, doctorID, specialtyID))

	for _, s := range slots {
		if s.IsBooked {
			busy++
			fmt.Fprintf(&b, "%s — занято (ID %d)\n", s.StartAt.Format("15:04"), s.ID)
			continue
		}
		if s.IsAvailable {
			free++
			fmt.Fprintf(&b, "%s — свободно (ID %d)\n", s.StartAt.Format("15:04"), s.ID)
		} else {
			closed++
			fmt.Fprintf(&b, "%s — закрыто (ID %d)\n", s.StartAt.Format("15:04"), s.ID)
		}
	}

	b.WriteString(fmt.Sprintf("\nИтого: свободно=%d, закрыто=%d, занято=%d", free, closed, busy))

	_ = s.repo.LogAdminAction(ctx, userID, "view_doctor_day_slots", fmt.Sprintf("doctor_id=%d specialty_id=%d date=%s slots=%d free=%d closed=%d busy=%d", doctorID, specialtyID, dateStr, len(slots), free, closed, busy))
	_ = s.repo.DeleteConversationState(ctx, userID)
	return true, strings.TrimSpace(b.String()), nil
}

func (s *BookingService) adminDictionaries(ctx context.Context) (string, error) {
	doctors, err := s.repo.ListAllDoctors(ctx)
	if err != nil {
		return "", err
	}
	specialties, err := s.repo.ListAllSpecialties(ctx)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("Врачи:\n")
	for _, d := range doctors {
		fmt.Fprintf(&b, "%d) %s\n", d.ID, d.FullName)
	}
	b.WriteString("\nСпециализации:\n")
	for _, s := range specialties {
		fmt.Fprintf(&b, "%d) %s\n", s.ID, s.Name)
	}
	return strings.TrimSpace(b.String()), nil
}

func parseClockToMinute(raw string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

func (s *BookingService) handleNameInput(ctx context.Context, userID int64, payload statePayload, text string) (bool, string, error) {
	name := strings.TrimSpace(text)
	if len(name) < 2 {
		return true, "Please enter a valid full name (at least 2 characters).", nil
	}
	payload.FullName = name
	if err := s.saveState(ctx, userID, StateWaitingPhone, payload); err != nil {
		return true, "", err
	}
	return true, "Great. Now send your phone number in international format, for example: +79991234567", nil
}

func (s *BookingService) handlePhoneInput(ctx context.Context, userID int64, payload statePayload, text string) (bool, string, error) {
	phone := normalizePhone(text)
	if !looksLikePhone(phone) {
		return true, "Please send a valid phone number, for example: +79991234567", nil
	}
	payload.Phone = phone
	if _, err := s.repo.UpsertClient(ctx, repository.Client{
		TelegramUserID: userID,
		FullName:       payload.FullName,
		Phone:          payload.Phone,
	}); err != nil {
		return true, "", err
	}
	if _, err := s.repo.EnsureUserProfile(ctx, userID); err != nil {
		return true, "", err
	}
	_ = s.repo.GrantReferralRewardsOnRegistration(ctx, userID, RefereeSignupBonusCents, ReferrerSignupBonusCents)
	if err := s.repo.DeleteConversationState(ctx, userID); err != nil {
		return true, "", err
	}
	return true, "Профиль сохранен. Нажмите \"🗓️ Записаться\", чтобы выбрать направление и врача.", nil
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
		"Confirm booking:\nName: %s\nPhone: %s\nService: %s\nSlot: %s\n\nReply YES to confirm or NO to cancel.",
		payload.FullName,
		payload.Phone,
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

	booking, err := s.repo.ConfirmServiceBooking(ctx, repository.Booking{
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

func normalizePhone(raw string) string {
	s := phoneCleaner.ReplaceAllString(strings.TrimSpace(raw), "")
	if strings.HasPrefix(s, "8") && len(s) == 11 {
		return "+7" + s[1:]
	}
	if strings.HasPrefix(s, "7") && len(s) == 11 {
		return "+" + s
	}
	return s
}

func looksLikePhone(phone string) bool {
	phone = strings.TrimPrefix(phone, "+")
	if len(phone) < 10 || len(phone) > 15 {
		return false
	}
	for _, ch := range phone {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (s *BookingService) IsRegistered(ctx context.Context, userID int64) (bool, error) {
	client, err := s.repo.GetClientByUserID(ctx, userID)
	if err == repository.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(client.FullName) != "" && strings.TrimSpace(client.Phone) != "", nil
}

func (s *BookingService) ListSpecialtiesPage(ctx context.Context, page, pageSize int) ([]repository.Specialty, int, error) {
	cacheKey := fmt.Sprintf("specpage:%d:%d", page, pageSize)
	if s.cache != nil {
		raw, err := s.cache.Get(ctx, cacheKey)
		if err == nil && raw != "" {
			var payload struct {
				Items []repository.Specialty `json:"items"`
				Total int                    `json:"total"`
			}
			if jsonErr := json.Unmarshal([]byte(raw), &payload); jsonErr == nil {
				return payload.Items, payload.Total, nil
			}
		}
	}

	offset := page * pageSize
	items, err := s.repo.ListSpecialties(ctx, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.CountSpecialties(ctx)
	if err != nil {
		return nil, 0, err
	}

	if s.cache != nil {
		payload, _ := json.Marshal(struct {
			Items []repository.Specialty `json:"items"`
			Total int                    `json:"total"`
		}{Items: items, Total: total})
		_ = s.cache.Set(ctx, cacheKey, string(payload), specialtyCacheTTL)
	}

	return items, total, nil
}

func (s *BookingService) ListDoctorsPage(ctx context.Context, specialtyID int64, page, pageSize int) ([]repository.Doctor, int, error) {
	offset := page * pageSize
	items, err := s.repo.ListDoctorsBySpecialty(ctx, specialtyID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.CountDoctorsBySpecialty(ctx, specialtyID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *BookingService) ListSlotsPage(ctx context.Context, specialtyID, doctorID int64, page, pageSize int) ([]repository.DoctorSlot, int, error) {
	offset := page * pageSize
	items, err := s.repo.ListAvailableDoctorSlots(ctx, specialtyID, doctorID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.CountAvailableDoctorSlots(ctx, specialtyID, doctorID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *BookingService) ConfirmClinicBooking(ctx context.Context, userID, specialtyID, doctorID, slotID int64, lang i18n.Lang) (string, error) {
	if _, err := s.repo.EnsureUserProfile(ctx, userID); err != nil {
		return "", err
	}

	slot, err := s.repo.GetDoctorSlotByID(ctx, slotID)
	if err != nil {
		return "", err
	}
	if !slot.IsAvailable || slot.DoctorID != doctorID || slot.SpecialtyID != specialtyID {
		return "Этот слот уже недоступен. Выберите другое время.", nil
	}

	opID := fmt.Sprintf("clinic_booking:confirm:%d:%d", userID, slotID)
	res, err := s.repo.ConfirmPaidClinicBooking(ctx, userID, ClinicBookingFeeCents, specialtyID, doctorID, slotID, opID)
	if err != nil {
		if errors.Is(err, repository.ErrInsufficientFunds) {
			p, perr := s.repo.GetUserProfile(ctx, userID)
			if perr != nil {
				return "", perr
			}
			return "", &InsufficientFundsError{FeeCents: ClinicBookingFeeCents, BalanceCents: p.BalanceCents}
		}
		if errors.Is(err, repository.ErrNotFound) {
			return "Этот слот уже занят. Выберите другое время.", nil
		}
		return "", err
	}

	b := i18n.Bundle{Lang: lang}
	return b.BookingConfirmed(
		res.BookingID,
		res.SpecialtyName,
		res.DoctorName,
		res.SlotStart.Format("02.01.2006 15:04"),
		ClinicBookingFeeCents,
		res.BalanceAfter,
	), nil
}

func (s *BookingService) ListClinicBookingsPage(ctx context.Context, userID int64, page, pageSize int) ([]repository.ClinicBookingView, int, error) {
	offset := page * pageSize
	items, err := s.repo.ListUserClinicBookings(ctx, userID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.CountUserClinicBookings(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *BookingService) CancelClinicBooking(ctx context.Context, userID, bookingID int64) (string, error) {
	result, err := s.repo.CancelClinicBooking(ctx, userID, bookingID)
	if err != nil {
		if err == repository.ErrNotFound {
			return "Запись не найдена.", nil
		}
		return "", err
	}
	msg := fmt.Sprintf(
		"Запись отменена.\nID: %d\nНаправление: %s\nВрач: %s\nВремя: %s",
		result.Booking.ID,
		result.Booking.SpecialtyName,
		result.Booking.DoctorName,
		result.Booking.StartAt.Format("02.01.2006 15:04"),
	)
	if result.RefundApplied {
		msg += fmt.Sprintf("\nВозврат: +%d коп.\nБаланс после возврата: %d коп.", result.RefundedCents, result.BalanceAfter)
	}
	if result.RefundBlockedByPolicy {
		msg += "\nВозврат недоступен: время приема уже началось."
	}
	return msg, nil
}

func (s *BookingService) SaveUploadedDocument(ctx context.Context, userID int64, fileID, fileName, mimeType string, fileSize int) (string, error) {
	st, err := s.repo.GetConversationState(ctx, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return "Сначала нажмите «📤 Загрузить документ».", nil
		}
		return "", err
	}
	if st.State != StateWaitingDocument {
		return "Сначала нажмите «📤 Загрузить документ».", nil
	}
	if strings.TrimSpace(fileID) == "" {
		return "Не удалось получить идентификатор файла. Попробуйте снова.", nil
	}
	_, err = s.repo.SaveUserDocument(ctx, repository.UserDocument{
		TelegramUserID: userID,
		FileID:         fileID,
		FileName:       strings.TrimSpace(fileName),
		MimeType:       strings.TrimSpace(mimeType),
		FileSize:       fileSize,
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		return "", err
	}
	_ = s.repo.DeleteConversationState(ctx, userID)
	recent, err := s.repo.ListRecentUserDocuments(ctx, userID, 3)
	if err != nil {
		return "", err
	}
	var names []string
	for _, d := range recent {
		name := d.FileName
		if strings.TrimSpace(name) == "" {
			name = "Без имени"
		}
		names = append(names, name)
	}
	return "Документ сохранен. Последние загрузки: " + strings.Join(names, ", "), nil
}
