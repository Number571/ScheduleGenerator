package schedule

import (
    "os"
    "sort"
    "time"
    "errors"
    "io/ioutil"
    "math/rand"
    "encoding/json"
)

func init() {
    rand.Seed(time.Now().UnixNano())
}

func (gen *Generator) tryGenerate(
    subgroup SubgroupType, 
    subjtype SubjectType, 
    group *Group, 
    subject *Subject, 
    schedule *Schedule, 
    countl *Subgroup, 
    template []*Schedule,
) {
    if gen.inBlockedGroups(group.Name) {
        return
    }
    var (
        indexGroup = -1
        flag       = false
    )
    if template != nil {
        for i, sch := range template {
            if sch.Group == group.Name {
                indexGroup = i
            }
        }
        if indexGroup == -1 {
            return
        }
    }
startAgain:
    nextLesson: for lesson := uint(0); lesson < NUM_TABLES; lesson++ {
        // Если лимит пар на день окончен, тогда прекратить ставить пары группе.
        if gen.isLimitLessons(subgroup, countl) {
            break nextLesson
        }

        // Если это полная группа и у неё не осталось теоретических занятий, тогда пропустить этот предмет. 
        if subjtype == THEORETICAL && !gen.haveTheoreticalLessons(subject) {
            break nextLesson
        }

        // Если это подгруппа и у неё не осталось практических занятий, тогда пропустить этот предмет.
        if subjtype == PRACTICAL && !gen.havePracticalLessons(subgroup, subject) {
            break nextLesson
        }

        // Если учитель заблокирован (не может присутствовать на занятиях) или
        // лимит пар на текущую неделю для предмета завершён, тогда пропустить этот предмет.
        if gen.inBlockedTeachers(subject.Teacher) || gen.notHaveWeekLessons(subgroup, subject) {
            break nextLesson
        }

        isAfter := false
        savedLesson := lesson

        // [ I ] Первая проверка.
        switch subgroup {
        case ALL:
            // Если две подгруппы стоят друг за другом, тогда исключить возможность добавления полной пары.
            for i := uint(0); i < NUM_TABLES-1; i++ {
                if  gen.cellIsReserved(A, schedule, i) && !gen.cellIsReserved(B, schedule, i) && 
                    gen.cellIsReserved(B, schedule, i+1) && !gen.cellIsReserved(A, schedule, i+1) ||
                    gen.cellIsReserved(B, schedule, i) && !gen.cellIsReserved(A, schedule, i) && 
                    gen.cellIsReserved(A, schedule, i+1) && !gen.cellIsReserved(B, schedule, i+1) {
                        break nextLesson
                    }
            }

            // Если между двумя разными подгруппами находятся окна, тогда посчитать сколько пустых окон.
            // Если их количество = 1, тогда попытаться подставить полную пару под это окно. 
            // Если не получается, тогда не ставить полную пару.
            cellSubgroupReserved := false
            indexNotReserved := uint(0)
            cellIsNotReserved := 0
            for i := uint(0); i < NUM_TABLES; i++ {
                if  gen.cellIsReserved(A, schedule, i) && !gen.cellIsReserved(B, schedule, i) ||
                    gen.cellIsReserved(B, schedule, i) && !gen.cellIsReserved(A, schedule, i) {
                        if cellIsNotReserved != 0 {
                            if cellIsNotReserved == 1 {
                                lesson = indexNotReserved
                                goto tryAfter
                            }
                            break nextLesson
                        }
                        cellSubgroupReserved = true
                    }
                if !gen.cellIsReserved(A, schedule, i) && !gen.cellIsReserved(B, schedule, i) && cellSubgroupReserved {
                    if cellIsNotReserved == 0 {
                        indexNotReserved = i
                    }
                    cellIsNotReserved += 1
                }
            }

            switch {
            case    lesson > 1 && (gen.cellIsReserved(A, schedule, lesson-1) || gen.cellIsReserved(B, schedule, lesson-1)) &&
                    !(gen.cellIsReserved(A, schedule, lesson+1) || gen.cellIsReserved(B, schedule, lesson+1)): // pass
            default: 
                // "Подтягивать" полные пары к уже существующим [перед].
                for i := uint(0); i < NUM_TABLES-1; i++ {
                    if  (gen.cellIsReserved(A, schedule, i+1) || gen.cellIsReserved(B, schedule, i+1)) &&
                        (!gen.cellIsReserved(A, schedule, i) && !gen.cellIsReserved(B, schedule, i)) {
                            lesson = i
                            break
                        }
                }
            }

        default:
            switch {
            case    lesson > 1 && gen.cellIsReserved(subgroup, schedule, lesson-1) &&
                    !gen.cellIsReserved(subgroup, schedule, lesson+1): // pass
            default: 
                // "Подтягивать" неполные пары к уже существующим [перед].
                for i := uint(0); i < NUM_TABLES-1; i++ {
                    if  !gen.cellIsReserved(subgroup, schedule, i) && gen.cellIsReserved(subgroup, schedule, i+1) {
                            lesson = i
                            break
                        }
                }
            }
        }

tryAfter:
        if isAfter {
            switch subgroup {
            case ALL:
                // "Подтягивать" полные пары к уже существующим [после].
                for i := uint(0); i < NUM_TABLES-1; i++ {
                    if  (gen.cellIsReserved(A, schedule, i) || gen.cellIsReserved(B, schedule, i)) &&
                        (!gen.cellIsReserved(A, schedule, i+1) && !gen.cellIsReserved(B, schedule, i+1)) {
                            lesson = i+1
                            break
                        }
                }
            default:
                // "Подтягивать" неполные пары к уже существующим [после].
                for i := uint(0); i < NUM_TABLES-1; i++ {
                    if  (gen.cellIsReserved(ALL, schedule, i) || gen.cellIsReserved(subgroup, schedule, i)) &&
                        !gen.cellIsReserved(subgroup, schedule, i+1) {
                            lesson = i+1
                            break
                        }
                }
            }
        }

        var (
            cabinet = ""
            cabinet2 = ""
        )
        // Если ячейка уже зарезервирована или учитель занят, или кабинет зарезервирован, или
        // если это полная пара и ячейка занята либо первой, либо второй подгруппой, или
        // если это двойная пара и кабинеты второго учителя зарезервированы, или 
        // если это двойная пара и второй учитель занят: попытаться сдвинуть пары к уже существующим.
        // Если не получается, тогда не ставить этот урок.
        if  gen.cellIsReserved(subgroup, schedule, lesson) || 
            gen.teacherIsReserved(subject.Teacher, lesson) || 
            gen.cabinetIsReserved(subgroup, subject, subject.Teacher, lesson, &cabinet) || 
            (subgroup == ALL && (gen.cellIsReserved(A, schedule, lesson) || gen.cellIsReserved(B, schedule, lesson))) ||
            (gen.withSubgroups(group.Name) && gen.isDoubleLesson(group.Name, subject.Name) && gen.teacherIsReserved(subject.Teacher2, lesson)) || 
            (gen.withSubgroups(group.Name) && gen.isDoubleLesson(group.Name, subject.Name) && gen.cabinetIsReserved(subgroup, subject, subject.Teacher2, lesson, &cabinet2)) {
                if isAfter {
                    break nextLesson
                }
                if lesson != savedLesson {
                    isAfter = true
                    goto tryAfter
                }
                continue nextLesson
        }

        // Если существует шаблон расписания, тогда придерживаться его структуры.
        if template != nil && template[indexGroup].Table[lesson].Subject[A] == "" && template[indexGroup].Table[lesson].Subject[B] == "" {
            if  (gen.cellIsReserved(A, schedule, lesson) && !gen.cellIsReserved(B, schedule, lesson)) ||
                (gen.cellIsReserved(B, schedule, lesson) && !gen.cellIsReserved(A, schedule, lesson)) {
                    goto passTemplate
            }
            lesson = savedLesson
            continue nextLesson
        }

passTemplate:
        // Полный день - максимум 7 пар.
        // lesson начинается с нуля!
        if (gen.Day != WEDNESDAY && gen.Day != SATURDAY) && lesson >= 7 {
            break nextLesson
        }

        // В среду и субботу - 5-6 пары должны быть свободны.
        // lesson начинается с нуля!
        if (gen.Day == WEDNESDAY || gen.Day == SATURDAY) && (lesson == 4 || lesson == 5) {
            lesson = savedLesson // Возобновить сохранённое занятие.
            continue nextLesson // Перейти к следующей ячейке.
        }

        // [ II ] Вторая проверка.
        switch subgroup {
        case ALL:
            // Если уже существует полная пара, которая стоит за парами с подгруппами, тогда
            // перейти на новую ячейку расписания группы.
            for i := lesson; i < NUM_TABLES-1; i++ {
                if  gen.cellIsReserved(A, schedule, i) && !gen.cellIsReserved(B, schedule, i) && gen.cellIsReserved(ALL, schedule, i+1) ||
                    gen.cellIsReserved(B, schedule, i) && !gen.cellIsReserved(A, schedule, i) && gen.cellIsReserved(ALL, schedule, i+1) {
                        lesson = savedLesson
                        continue nextLesson
                    }
            }
        default:
            // Если у одной подгруппы уже имеется пара, а у второй стоит пара
            // в это же время, тогда пропустить проверку пустых окон.
            if  gen.cellIsReserved(A, schedule, lesson) && A != subgroup || 
                gen.cellIsReserved(B, schedule, lesson) && B != subgroup {
                    goto passCheck
            }
            // Если стоит полная пара, а за ней идёт подгруппа неравная проверяемой, тогда
            // прекратить ставить пары у проверяемой подгруппы.
            fullLessons := false
            for i := uint(0); i < lesson; i++ {
                if gen.cellIsReserved(ALL, schedule, i) {
                    fullLessons = true
                    continue
                }
                if  (fullLessons && gen.cellIsReserved(A, schedule, i) && A != subgroup) ||
                    (fullLessons && gen.cellIsReserved(B, schedule, i) && B != subgroup) {
                        break nextLesson
                }
            }
        }

passCheck:
        // [ III ] Третья проверка.
        // Если нет возможности добавить новые пары без создания окон, тогда не ставить пары.
        if lesson > 1 {
            switch subgroup {
            case ALL:
                for i := uint(0); i < lesson-1; i++ {
                    if  (gen.cellIsReserved(A, schedule, i) && !gen.cellIsReserved(A, schedule, lesson-1)) || 
                        (gen.cellIsReserved(B, schedule, i) && !gen.cellIsReserved(B, schedule, lesson-1)) {
                            break nextLesson
                    }
                }
            default:
                for i := uint(0); i < lesson-1; i++ {
                    if  gen.cellIsReserved(subgroup, schedule, i) && !gen.cellIsReserved(subgroup, schedule, lesson-1) {
                        break nextLesson
                    }
                }
            }
        }

        gen.reserved.teachers[subject.Teacher][lesson] = true
        gen.reserved.cabinets[cabinet][lesson] = true

        switch subgroup {
        case A:
            gen.Groups[group.Name].Subjects[subject.Name].WeekLessons.A -= 1
            gen.Groups[group.Name].Subjects[subject.Name].Practice.A -= 1
            countl.A += 1
        case B:
            gen.Groups[group.Name].Subjects[subject.Name].WeekLessons.B -= 1
            gen.Groups[group.Name].Subjects[subject.Name].Practice.B -= 1
            countl.B += 1
        case ALL:
            gen.Groups[group.Name].Subjects[subject.Name].WeekLessons.A -= 1
            gen.Groups[group.Name].Subjects[subject.Name].WeekLessons.B -= 1
            countl.A += 1
            countl.B += 1
            switch subjtype {
            case THEORETICAL:
                gen.Groups[group.Name].Subjects[subject.Name].Theory -= 1
            case PRACTICAL:
                gen.Groups[group.Name].Subjects[subject.Name].Practice.A -= 1
                gen.Groups[group.Name].Subjects[subject.Name].Practice.B -= 1
            }
        }

        if subgroup == ALL {
            schedule.Table[lesson].Teacher = [ALL]string{
                subject.Teacher,
                subject.Teacher,
            }
            schedule.Table[lesson].Subject = [ALL]string{
                subject.Name,
                subject.Name,
            }
            schedule.Table[lesson].Cabinet = [ALL]string{
                cabinet,
                cabinet,
            }
            // Если это двойная пара и группа делимая, тогда поставить пару с разными преподавателями.
            if gen.isDoubleLesson(group.Name, subject.Name) && gen.withSubgroups(group.Name) {
                gen.reserved.teachers[subject.Teacher2][lesson] = true
                gen.reserved.cabinets[cabinet2][lesson] = true
                schedule.Table[lesson].Teacher[B] = subject.Teacher2
                schedule.Table[lesson].Cabinet[B] = cabinet2
            }
            lesson = savedLesson
            continue nextLesson
        }

        schedule.Table[lesson].Teacher[subgroup] = subject.Teacher
        schedule.Table[lesson].Subject[subgroup] = subject.Name
        schedule.Table[lesson].Cabinet[subgroup] = cabinet
        lesson = savedLesson
    }

    if template != nil && !flag {
        flag = true
        goto startAgain
    }
}

func (gen *Generator) isLimitLessons(subgroup SubgroupType, countl *Subgroup) bool {
    switch subgroup {
    case ALL:
        if countl.A >= MAX_COUNT_LESSONS_IN_DAY && countl.B >= MAX_COUNT_LESSONS_IN_DAY {
            return true
        }
    case A:
        if countl.A >= MAX_COUNT_LESSONS_IN_DAY {
            return true
        }
    case B:
        if countl.B >= MAX_COUNT_LESSONS_IN_DAY {
            return true
        }
    }
    return false
}

func (gen *Generator) withSubgroups(group string) bool {
    if gen.Groups[group].Quantity > MAX_COUNT_WITHOUT_SUBGROUPS {
        return true
    }
    return false
}

func remove(data []string, index int) []string {
    return append(data[:index], data[index+1:]...)
}

func (gen *Generator) blockGroup(group string) error {
    if !gen.inGroups(group) {
        return errors.New("group undefined")
    }
    // gen.Blocked.Groups[group] = true
    for _, elem := range gen.Blocked.Groups {
        if elem == group {
            return nil
        }
    }
    gen.Blocked.Groups = append(gen.Blocked.Groups, group)
    return nil
}

func (gen *Generator) unblockGroup(group string) error {
    if !gen.inBlockedGroups(group) {
        return errors.New("group undefined")
    }
    // delete(gen.Blocked.Groups, group)
    for i, elem := range gen.Blocked.Groups {
        if elem == group {
            remove(gen.Blocked.Groups, i)
            break
        }
    }
    return nil
}

func (gen *Generator) inBlockedGroups(group string) bool {
    // if _, ok := gen.Blocked.Groups[group]; !ok {
    //     return false
    // }
    // return true
    for _, elem := range gen.Blocked.Groups {
        if elem == group {
            return true
        }
    }
    return false
}

func (gen *Generator) blockTeacher(teacher string) error {
    if !gen.inTeachers(teacher) {
        return errors.New("teacher undefined")
    }
    // gen.Blocked.Teachers[teacher] = true
    for _, elem := range gen.Blocked.Teachers {
        if elem == teacher {
            return nil
        }
    }
    gen.Blocked.Teachers = append(gen.Blocked.Teachers, teacher)
    return nil
}

func (gen *Generator) unblockTeacher(teacher string) error {
    if !gen.inBlockedTeachers(teacher) {
        return errors.New("teacher undefined")
    }
    // delete(gen.Blocked.Teachers, teacher)
    for i, elem := range gen.Blocked.Teachers {
        if elem == teacher {
            remove(gen.Blocked.Teachers, i)
            break
        }
    }
    return nil
}

func (gen *Generator) inBlockedTeachers(teacher string) bool {
    // if _, ok := gen.Blocked.Teachers[teacher]; !ok {
    //     return false
    // }
    // return true
    for _, elem := range gen.Blocked.Teachers {
        if elem == teacher {
            return true
        }
    }
    return false
}

func (gen *Generator) inGroups(group string) bool {
    if _, ok := gen.Groups[group]; !ok {
        return false
    }
    return true
}

func (gen *Generator) inTeachers(teacher string) bool {
    if _, ok := gen.Teachers[teacher]; !ok {
        return false
    }
    return true
}

func packJSON(data interface{}) []byte {
    jsonData, err := json.MarshalIndent(data, "", "\t")
    if err != nil {
        return nil
    }
    return jsonData
}

func unpackJSON(data []byte, output interface{}) error {
    err := json.Unmarshal(data, output)
    return err
}

func writeJSON(filename string, data interface{}) error {
    return writeFile(filename, string(packJSON(data)))
}

func (gen *Generator) subjectInGroup(subject string, group string) bool {
    if !gen.inGroups(group) {
        return false
    }
    if _, ok := gen.Groups[group].Subjects[subject]; !ok {
        return false
    }
    return true
}

func (gen *Generator) isDoubleLesson(group string, subject string) bool {
    if !gen.inGroups(group) {
        return false
    }
    if _, ok := gen.Groups[group].Subjects[subject]; !ok {
        return false
    }
    if gen.Groups[group].Subjects[subject].Teacher2 == "" {
        return false
    }
    return true
}

func shuffle(slice interface{}) interface{}{
    switch slice.(type) {
    case []*Group:
        result := slice.([]*Group)
        for i := len(result)-1; i > 0; i-- {
            j := rand.Intn(i+1)
            result[i], result[j] = result[j], result[i]
        }
        return result
    case []*Subject:
        result := slice.([]*Subject)
        for i := len(result)-1; i > 0; i-- {
            j := rand.Intn(i+1)
            result[i], result[j] = result[j], result[i]
        }
        return result
    }
    return nil
}

func (gen *Generator) cellIsReserved(subgroup SubgroupType, schedule *Schedule, lesson uint) bool {
    if lesson >= NUM_TABLES {
        return false
    }
    switch subgroup {
    case A: 
        if schedule.Table[lesson].Subject[A] != "" {
            return true
        }
    case B:
        if schedule.Table[lesson].Subject[B] != "" {
            return true
        }
    case ALL:
        if schedule.Table[lesson].Subject[A] != "" && schedule.Table[lesson].Subject[B] != "" {
            return true
        }
    }
    return false
}

func (gen *Generator) cabinetIsReserved(subgroup SubgroupType, subject *Subject, teacher string, lesson uint, cabinet *string) bool {
    var result = true
    if !gen.inTeachers(teacher) {
        return result
    }
    for _, cab := range gen.Teachers[teacher].Cabinets {
        gen.cabinetToReserved(cab.Name)
        // Если это не компьютерный кабинет, а предмет предполагает практику в компьютерных кабинетах и идёт время практики,
        // тогда посмотреть другой кабинет преподавателя.
        if   subject.IsComputer && !cab.IsComputer &&
             gen.havePracticalLessons(subgroup, subject) &&
            !gen.haveTheoreticalLessons(subject) {
                continue
        }
        // Если кабинет не зарезирвирован, тогда занять кабинет.
        if _, ok := gen.reserved.cabinets[cab.Name]; ok && !gen.reserved.cabinets[cab.Name][lesson] {
            *cabinet = cab.Name
            return false
        }
    }
    return result
}

func (gen *Generator) cabinetToReserved(cabnum string) {
    if _, ok := gen.reserved.cabinets[cabnum]; ok {
        return
    }
    gen.reserved.cabinets[cabnum] = make([]bool, NUM_TABLES)
}

func (gen *Generator) teacherIsReserved(teacher string, lesson uint) bool {
    gen.teacherToReserved(teacher)
    if value, ok := gen.reserved.teachers[teacher]; ok {
        return value[lesson] == true
    }
    return false
}

func (gen *Generator) teacherToReserved(teacher string) {
    if _, ok := gen.reserved.teachers[teacher]; ok {
        return
    }
    gen.reserved.teachers[teacher] = make([]bool, NUM_TABLES)
}

func (gen *Generator) notHaveWeekLessons(subgroup SubgroupType, subject *Subject) bool {
    switch subgroup {
    case A:
        if subject.WeekLessons.A == 0 {
            return true
        }
    case B:
        if subject.WeekLessons.B == 0 {
            return true
        }
    case ALL:
        if subject.WeekLessons.A == 0 && subject.WeekLessons.B == 0 {
            return true
        }
    }
    return false
}

func (gen *Generator) haveTheoreticalLessons(subject *Subject) bool {
    if subject.Theory == 0 {
        return false
    }
    return true
}

func (gen *Generator) havePracticalLessons(subgroup SubgroupType, subject *Subject) bool {
    switch subgroup {
    case A:
        if subject.Practice.A == 0 {
            return false
        }
    case B:
        if subject.Practice.B == 0 {
            return false
        }
    case ALL:
        if subject.Practice.A == 0 && subject.Practice.B == 0 {
            return false
        }
    }
    return true
}

func getGroups(groups map[string]*Group) []*Group {
    var list []*Group
    for _, group := range groups {
        list = append(list, group)
    }
    return shuffle(list).([]*Group)
}

func getSubjects(subjects map[string]*Subject) []*Subject {
    var list []*Subject
    for _, subject := range subjects {
        list = append(list, subject)
    }
    return shuffle(list).([]*Subject)
}

func sortSchedule(schedule []*Schedule) []*Schedule {
    sort.SliceStable(schedule, func(i, j int) bool {
        return schedule[i].Group < schedule[j].Group
    })
    return schedule
}

// // For xlsx
// func colWidthForCabinets(index int) (int, int, float64) {
//     var col = (index+1)*3+1
//     return col, col, COL_W_CAB
// }

// Returns [min:max] value.
func random(min, max int) int {
    return rand.Intn(max - min + 1) + min
}

func writeFile(filename, data string) error {
    file, err := os.Create(filename)
    if err != nil {
        return err
    }
    file.WriteString(data)
    file.Close()
    return nil
}

func readFile(filename string) string {
    data, err := ioutil.ReadFile(filename)
    if err != nil {
        return ""
    }
    return string(data)
}
