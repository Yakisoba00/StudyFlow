package main

import (
	"encoding/json"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// --- Структуры для парсинга JSON ---
type Lesson struct {
	Number          int    `json:"number"`
	LessonName      string `json:"lessonName"`
	TeacherName     string `json:"teacherName"`
	AuditoryName    string `json:"auditoryName"`
	TimeRange       string `json:"timeRange"`
	StartAt         string `json:"startAt"`
	EndAt           string `json:"endAt"`
	IsDistant       bool   `json:"isDistant"`
	Type            int    `json:"type"`
	Duration        int    `json:"duration"`
	DurationMinutes int    `json:"durationMinutes"`
}

type DayInfo struct {
	Type       int    `json:"type"`
	WeekNumber int    `json:"weekNumber"`
	Date       string `json:"date"`
}

type Day struct {
	Info    DayInfo  `json:"info"`
	Lessons []Lesson `json:"lessons"`
}

type Week struct {
	Number int   `json:"number"`
	Days   []Day `json:"days"`
}

type ScheduleResponse struct {
	IsCache bool   `json:"isCache"`
	Items   []Week `json:"items"`
}

// --- Внутренние структуры для отображения ---
type DisplayLesson struct {
	Number       int
	TimeRange    string
	LessonName   string
	TeacherName  string
	AuditoryName string
	IsDistant    bool
	Type         int
	Duration     int
}

type DaySchedule struct {
	Date    time.Time
	Lessons []DisplayLesson
}

type CalendarWeek struct {
	Number    int                           // номер недели в году
	StartDate time.Time                     // понедельник
	EndDate   time.Time                     // воскресенье
	Days      map[time.Weekday]*DaySchedule // ключ - день недели
}

type Semester struct {
	Name      string
	StartDate time.Time
	EndDate   time.Time
	Weeks     []*CalendarWeek
}

// rawLesson используется при первичной обработке
type rawLesson struct {
	date   time.Time
	lesson DisplayLesson
}

// --- Основное приложение ---
type ScheduleApp struct {
	window          fyne.Window
	groupEntry      *widget.Entry
	semesterSelect  *widget.Select
	weekSelect      *widget.Select
	contentCard     *widget.Card
	semesters       []*Semester
	currentSemester int
}

func main() {
	a := app.New()
	a.Settings().SetTheme(theme.DarkTheme())

	appInstance := &ScheduleApp{
		semesters: []*Semester{},
	}

	w := a.NewWindow("StudyFlow Расписание ЯГТУ")
	w.Resize(fyne.NewSize(1000, 750))
	w.CenterOnScreen()

	appInstance.window = w
	appInstance.setupUI()

	w.SetContent(appInstance.createMainLayout())
	w.ShowAndRun()
}

func (s *ScheduleApp) setupUI() {
	s.groupEntry = widget.NewEntry()
	s.groupEntry.SetPlaceHolder("Введите номер группы, например: ЦИС-37")
	s.groupEntry.SetIcon(theme.SearchIcon())

	s.semesterSelect = widget.NewSelect([]string{}, func(selected string) {
		for i, sem := range s.semesters {
			if sem.Name == selected {
				s.currentSemester = i
				s.updateWeekSelect()
				if len(sem.Weeks) > 0 {
					s.showWeekSchedule(0)
				}
				break
			}
		}
	})
	s.semesterSelect.PlaceHolder = "Выберите семестр"
	s.semesterSelect.Disable()

	s.weekSelect = widget.NewSelect([]string{}, func(selected string) {
		for i, week := range s.semesters[s.currentSemester].Weeks {
			weekStr := fmt.Sprintf("📅 Неделя %d (%s — %s)", week.Number,
				week.StartDate.Format("02.01"), week.EndDate.Format("02.01"))
			if weekStr == selected {
				s.showWeekSchedule(i)
				break
			}
		}
	})
	s.weekSelect.PlaceHolder = "Выберите неделю"
	s.weekSelect.Disable()

	s.contentCard = widget.NewCard("Расписание", "",
		widget.NewLabel("📚 Здесь появится расписание\n\n1️⃣ Введите номер группы\n2️⃣ Нажмите 'Загрузить расписание'\n3️⃣ Выберите семестр и неделю"))
}

func (s *ScheduleApp) createMainLayout() fyne.CanvasObject {
	loadBtn := widget.NewButtonWithIcon("Загрузить расписание", theme.DownloadIcon(), func() {
		go s.loadSchedule()
	})
	loadBtn.Importance = widget.HighImportance

	topPanel := container.NewVBox(
		widget.NewLabelWithStyle("📖 StudyFlow — Расписание ЯГТУ", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.NewVBox(
			s.groupEntry,
			container.NewCenter(loadBtn),
		),
		widget.NewSeparator(),
		container.NewCenter(
			container.NewHBox(
				widget.NewIcon(theme.NavigateNextIcon()),
				s.semesterSelect,
			),
		),
		container.NewCenter(
			container.NewHBox(
				widget.NewIcon(theme.NavigateNextIcon()),
				s.weekSelect,
			),
		),
		widget.NewSeparator(),
	)

	return container.NewBorder(
		topPanel,
		nil,
		nil,
		nil,
		container.NewVScroll(s.contentCard),
	)
}

func (s *ScheduleApp) loadSchedule() {
	group := strings.TrimSpace(s.groupEntry.Text)
	if group == "" {
		fyne.Do(func() {
			dialog.ShowInformation("Ошибка", "Пожалуйста, введите номер группы", s.window)
		})
		return
	}

	fyne.Do(func() {
		s.contentCard.SetSubTitle("Загрузка...")
		s.contentCard.SetContent(widget.NewLabelWithStyle("⏳ Получение расписания для группы "+group,
			fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
		s.semesterSelect.Disable()
		s.weekSelect.Disable()
	})

	url := fmt.Sprintf("https://gg-api.ystuty.ru/s/schedule/v1/schedule/group/%s", strings.ToUpper(group))
	resp, err := http.Get(url)
	if err != nil {
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("Ошибка подключения: %v", err), s.window)
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("Ошибка чтения: %v", err), s.window)
		})
		return
	}

	var scheduleResp ScheduleResponse
	if err := json.Unmarshal(body, &scheduleResp); err != nil {
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("Ошибка обработки данных: %v", err), s.window)
		})
		return
	}

	if len(scheduleResp.Items) == 0 {
		fyne.Do(func() {
			dialog.ShowInformation("Нет данных", "Расписание не найдено для группы "+group, s.window)
		})
		return
	}

	s.processScheduleData(scheduleResp.Items, group)
}

func (s *ScheduleApp) processScheduleData(weeks []Week, group string) {
	// 1. Собрать все занятия с реальными датами
	var rawLessons []rawLesson

	for _, week := range weeks {
		for _, day := range week.Days {
			parsedDate, err := time.Parse("2006-01-02T15:04:05.000Z", day.Info.Date)
			if err != nil {
				parsedDate, _ = time.Parse("2006-01-02", day.Info.Date[:10])
			}
			// Приводим к локальному часовому поясу
			parsedDate = parsedDate.In(time.Local)

			for _, lesson := range day.Lessons {
				if lesson.LessonName == "" || lesson.LessonName == "null" || lesson.LessonName == "None" {
					continue
				}
				rawLessons = append(rawLessons, rawLesson{
					date: parsedDate,
					lesson: DisplayLesson{
						Number:       lesson.Number,
						TimeRange:    lesson.TimeRange,
						LessonName:   lesson.LessonName,
						TeacherName:  lesson.TeacherName,
						AuditoryName: lesson.AuditoryName,
						IsDistant:    lesson.IsDistant,
						Type:         lesson.Type,
						Duration:     lesson.Duration,
					},
				})
			}
		}
	}

	// Сортировка по дате
	sort.Slice(rawLessons, func(i, j int) bool {
		return rawLessons[i].date.Before(rawLessons[j].date)
	})

	// 2. Разделить на семестры (осенний/весенний)
	s.semesters = s.splitBySemester(rawLessons)

	// 3. Обновить UI
	semesterNames := make([]string, len(s.semesters))
	for i, sem := range s.semesters {
		semesterNames[i] = sem.Name
	}
	fyne.Do(func() {
		s.semesterSelect.Options = semesterNames
		s.semesterSelect.Enable()
		if len(semesterNames) > 0 {
			s.semesterSelect.Selected = semesterNames[0]
			s.semesterSelect.Refresh()
			s.currentSemester = 0
			s.updateWeekSelect()
			if len(s.semesters[0].Weeks) > 0 {
				s.showWeekSchedule(0)
			}
		}
		s.contentCard.SetSubTitle(fmt.Sprintf("🎓 Группа %s", group))
	})
}

// splitBySemester группирует занятия по семестрам и внутри каждого строит недели
func (s *ScheduleApp) splitBySemester(rawLessons []rawLesson) []*Semester {
	if len(rawLessons) == 0 {
		return []*Semester{}
	}

	var semesters []*Semester
	var currentSemester *Semester
	var currentLessons []rawLesson

	for _, rl := range rawLessons {
		month := rl.date.Month()
		isAutumn := month >= time.September && month <= time.December
		isSpring := month >= time.February && month <= time.June

		semesterName := ""
		if isAutumn {
			semesterName = fmt.Sprintf("🍂 Осенний семестр %d", rl.date.Year())
		} else if isSpring {
			semesterName = fmt.Sprintf("🌸 Весенний семестр %d", rl.date.Year())
		} else {
			// январь, июль, август – прикрепляем к текущему семестру
			if currentSemester != nil {
				semesterName = currentSemester.Name
			} else {
				semesterName = fmt.Sprintf("📚 Семестр %d", rl.date.Year())
			}
		}

		if currentSemester == nil || semesterName != currentSemester.Name {
			// завершить предыдущий семестр
			if currentSemester != nil {
				currentSemester.Weeks = s.buildWeeks(currentLessons)
				semesters = append(semesters, currentSemester)
			}
			// начать новый
			currentSemester = &Semester{
				Name:      semesterName,
				StartDate: rl.date,
				EndDate:   rl.date,
				Weeks:     []*CalendarWeek{},
			}
			currentLessons = []rawLesson{}
		}
		currentLessons = append(currentLessons, rl)
		if rl.date.After(currentSemester.EndDate) {
			currentSemester.EndDate = rl.date
		}
	}
	if currentSemester != nil {
		currentSemester.Weeks = s.buildWeeks(currentLessons)
		semesters = append(semesters, currentSemester)
	}
	return semesters
}

// buildWeeks создаёт список календарных недель из занятий
func (s *ScheduleApp) buildWeeks(lessons []rawLesson) []*CalendarWeek {
	// Мапа: ключ - год+неделя
	type weekKey struct {
		year int
		week int
	}
	weeksMap := make(map[weekKey]*CalendarWeek)

	for _, rl := range lessons {
		year, weekNum := rl.date.ISOWeek()
		key := weekKey{year, weekNum}

		week, ok := weeksMap[key]
		if !ok {
			// Вычислить начало недели (понедельник)
			startDate := getWeekStart(rl.date)
			endDate := startDate.AddDate(0, 0, 6)
			week = &CalendarWeek{
				Number:    weekNum,
				StartDate: startDate,
				EndDate:   endDate,
				Days:      make(map[time.Weekday]*DaySchedule),
			}
			weeksMap[key] = week
		}

		weekday := rl.date.Weekday()
		if _, exists := week.Days[weekday]; !exists {
			week.Days[weekday] = &DaySchedule{
				Date:    rl.date,
				Lessons: []DisplayLesson{},
			}
		}
		week.Days[weekday].Lessons = append(week.Days[weekday].Lessons, rl.lesson)
	}

	// Сортировка занятий внутри каждого дня по номеру
	for _, week := range weeksMap {
		for _, day := range week.Days {
			sort.Slice(day.Lessons, func(i, j int) bool {
				return day.Lessons[i].Number < day.Lessons[j].Number
			})
		}
	}

	// Преобразовать мапу в слайс и отсортировать по дате
	result := make([]*CalendarWeek, 0, len(weeksMap))
	for _, w := range weeksMap {
		result = append(result, w)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartDate.Before(result[j].StartDate)
	})
	return result
}

func getWeekStart(date time.Time) time.Time {
	// переводим к понедельнику
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return date.AddDate(0, 0, -weekday+1)
}

func (s *ScheduleApp) updateWeekSelect() {
	if s.currentSemester >= len(s.semesters) {
		return
	}
	weeks := s.semesters[s.currentSemester].Weeks
	options := make([]string, len(weeks))
	for i, w := range weeks {
		options[i] = fmt.Sprintf("📅 Неделя %d (%s — %s)", w.Number,
			w.StartDate.Format("02.01"), w.EndDate.Format("02.01"))
	}
	fyne.Do(func() {
		s.weekSelect.Options = options
		s.weekSelect.Enable()
		if len(options) > 0 {
			s.weekSelect.Selected = options[0]
			s.weekSelect.Refresh()
		}
	})
}

// showWeekSchedule отображает выбранную неделю, показывая ВСЕ дни (ПН–ВС)
func (s *ScheduleApp) showWeekSchedule(idx int) {
	if s.currentSemester >= len(s.semesters) {
		return
	}
	weeks := s.semesters[s.currentSemester].Weeks
	if idx < 0 || idx >= len(weeks) {
		return
	}
	week := weeks[idx]

	content := container.NewVBox()

	// Заголовок недели
	header := widget.NewCard(
		"",
		"",
		container.NewVBox(
			widget.NewLabelWithStyle(fmt.Sprintf("📅 %d НЕДЕЛЯ", week.Number),
				fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(fmt.Sprintf("%s — %s",
				week.StartDate.Format("02.01.2006"), week.EndDate.Format("02.01.2006")),
				fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
		),
	)
	content.Add(header)
	content.Add(widget.NewSeparator())

	// Порядок дней: ПН, ВТ, СР, ЧТ, ПТ, СБ, ВС
	dayOrder := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday,
		time.Friday, time.Saturday, time.Sunday}
	dayNames := map[time.Weekday]string{
		time.Monday:    "Понедельник",
		time.Tuesday:   "Вторник",
		time.Wednesday: "Среда",
		time.Thursday:  "Четверг",
		time.Friday:    "Пятница",
		time.Saturday:  "Суббота",
		time.Sunday:    "Воскресенье",
	}
	dayIcons := map[time.Weekday]string{
		time.Monday:    "🌙",
		time.Tuesday:   "🔥",
		time.Wednesday: "💧",
		time.Thursday:  "🌳",
		time.Friday:    "⭐",
		time.Saturday:  "🎉",
		time.Sunday:    "😴",
	}

	for _, dow := range dayOrder {
		// Вычисляем дату для этого дня в данной неделе
		offset := (int(dow) - int(time.Monday) + 7) % 7
		dayDate := week.StartDate.AddDate(0, 0, offset)
		daySchedule, hasLessons := week.Days[dow]

		var dayCard *widget.Card
		if hasLessons && len(daySchedule.Lessons) > 0 {
			dayCard = widget.NewCard(
				fmt.Sprintf("%s %s • %s", dayIcons[dow], dayNames[dow], formatDateToRussian(dayDate)),
				fmt.Sprintf("📚 %d пар", len(daySchedule.Lessons)),
				s.createDayContent(daySchedule.Lessons),
			)
		} else {
			emptyContent := container.NewVBox(
				widget.NewLabelWithStyle("—", fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
			)
			dayCard = widget.NewCard(
				fmt.Sprintf("%s %s • %s", dayIcons[dow], dayNames[dow], formatDateToRussian(dayDate)),
				"📚 Нет пар",
				emptyContent,
			)
		}
		content.Add(dayCard)
		content.Add(widget.NewSeparator())
	}

	fyne.Do(func() {
		s.contentCard.SetContent(container.NewVScroll(content))
	})
}

func (s *ScheduleApp) createDayContent(lessons []DisplayLesson) fyne.CanvasObject {
	boxes := make([]fyne.CanvasObject, 0, len(lessons))
	for i, lesson := range lessons {
		lessonType := getLessonType(lesson.Type)
		typeEmoji := map[string]string{
			"лек": "📖",
			"пр":  "💻",
			"лаб": "🔬",
		}[lessonType]
		if typeEmoji == "" {
			typeEmoji = "📝"
		}

		durationText := fmt.Sprintf("⏱️ %d часа", lesson.Duration)
		if lesson.Duration <= 2 {
			durationText = "⏱️ 2 часа"
		}

		info := ""
		if lesson.AuditoryName != "" {
			info += fmt.Sprintf("📍 %s", lesson.AuditoryName)
		}
		if lesson.TeacherName != "" {
			if info != "" {
				info += " • "
			}
			info += fmt.Sprintf("👨‍🏫 %s", lesson.TeacherName)
		}
		if lesson.IsDistant {
			if info != "" {
				info += " • "
			}
			info += "🌐 Дистанционно"
		}
		if info != "" {
			info += " • " + durationText
		} else {
			info = durationText
		}

		lessonCard := container.NewVBox(
			container.NewHBox(
				widget.NewLabelWithStyle(fmt.Sprintf("%s %d.", typeEmoji, lesson.Number),
					fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				widget.NewLabelWithStyle(lesson.TimeRange,
					fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			),
			widget.NewLabelWithStyle(lesson.LessonName,
				fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(info,
				fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		)
		boxes = append(boxes, lessonCard)
		if i < len(lessons)-1 {
			boxes = append(boxes, widget.NewSeparator())
		}
	}
	return container.NewVBox(boxes...)
}

func formatDateToRussian(t time.Time) string {
	months := map[time.Month]string{
		time.January: "января", time.February: "февраля", time.March: "марта",
		time.April: "апреля", time.May: "мая", time.June: "июня",
		time.July: "июля", time.August: "августа", time.September: "сентября",
		time.October: "октября", time.November: "ноября", time.December: "декабря",
	}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()], t.Year())
}

func getLessonType(t int) string {
	switch t {
	case 1:
		return "лек"
	case 2:
		return "пр"
	case 4:
		return "лаб"
	case 8:
		return "конс"
	case 16:
		return "зач"
	case 32:
		return "экз"
	case 64:
		return "курс"
	case 128:
		return "док"
	case 256:
		return "срс"
	case 512:
		return "инд"
	case 1024:
		return "конс"
	case 2048:
		return "пр"
	case 4096:
		return "вн"
	default:
		return "зан"
	}
}
