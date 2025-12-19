package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Configuration Constants ---
const (
	DateFormat          = "2006-01-02"
	FileExtension       = ".org"
	SourceHeader        = "* For tomorrow"
	DestinationHeader   = "* TODO"
	ConfigDirName       = "goday"
	ConfigFileName      = "projects.json"
	DaemonSpinUpTime    = 20
	DaemonOpenRetryTime = 5
	DoomLoadedMsg       = "Doom loaded "
	SuccessMsg          = "Success"
)

// --- Styles (Lipgloss) ---
var (
	// Colors
	normal    = lipgloss.AdaptiveColor{Light: "#EEE", Dark: "#222"}
	subtle    = lipgloss.AdaptiveColor{Light: "#a3a59dff", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	warning   = lipgloss.AdaptiveColor{Light: "#F25D94", Dark: "#F55385"}
	gold      = lipgloss.Color("#F1C40F")

	// Global Layout
	docStyle = lipgloss.NewStyle().Margin(1, 2)

	// text elements
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(highlight).
			Padding(0, 1).
			Bold(true)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(highlight).
				Bold(true).
		// Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(highlight).
		PaddingLeft(1)

	unselectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				PaddingLeft(2) // Indent to match border of selected

	pathStyle = lipgloss.NewStyle().
			Foreground(subtle).
			Italic(true)

	//I could aslo create a "selectedStartStyle" to have it more cojoined with selected
	//Then I'd need non-starred selected and starred selected separetely
	starStyle = lipgloss.NewStyle().Foreground(gold)

	normalStyle  = lipgloss.NewStyle().Foreground(normal)
	successStyle = lipgloss.NewStyle().Foreground(special)
	failureStyle = lipgloss.NewStyle().Foreground(warning)

	helpStyle = lipgloss.NewStyle().
			Foreground(subtle).
			MarginTop(1)
)

// --- Data Models ---
type Project struct {
	Name                string    `json:"name"`
	Path                string    `json:"path"`
	Starred             bool      `json:"starred"`
	LastOpened          time.Time `json:"last_opened"`
	LastFileCreated     string    `json:"last_file_created"`
	PreviousFileCreated string    `json:"previous_file_created"`
}

// --- Bubble Tea Model ---
type model struct {
	projects      []Project
	cursor        int
	addingNew     bool
	textInput     textinput.Model
	selectedPath  string
	width, height int
	daemonSpunUp  bool
}

type DaemonReadyMsg struct{}
type StartDaemonMsg struct{}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "/absolute/path/to/project"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 50
	// Style the input prompt
	ti.PromptStyle = lipgloss.NewStyle().Foreground(highlight)

	return model{
		projects:  loadConfig(),
		textInput: ti,
		cursor:    0,
	}
}

func (m model) Init() tea.Cmd {
	lipgloss.SetHasDarkBackground(false)
	return func() tea.Msg {
		return StartDaemonMsg{}
	}
}

// --- Update Loop ---
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case StartDaemonMsg:
		return m, func() tea.Msg {
			fmt.Println("xd")
			ch := make(chan string)
			go func() {
				runEmacsDaemon(ch)
			}()
			emacsRunResult := <-ch
			if emacsRunResult == SuccessMsg {
				return DaemonReadyMsg{}
			}
			return nil
		}

	case DaemonReadyMsg:
		m.daemonSpunUp = true
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		// Input Mode
		if m.addingNew {
			switch msg.Type {
			case tea.KeyEnter:
				path := m.textInput.Value()
				if path != "" {
					m.addProject(path)
					m.textInput.Reset()
					m.addingNew = false
				}
				return m, nil
			case tea.KeyEsc:
				m.textInput.Reset()
				m.addingNew = false
				return m, nil
			}
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

		// Navigation Mode
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case "a":
			m.addingNew = true
			return m, nil
		case "d", "x":
			if len(m.projects) > 0 {
				// m.projects = append(m.projects[:m.cursor], m.projects[m.cursor+1:]...) //Remove element (take array before elemnt and add elements after the targeted element)
				m.projects = slices.Delete(m.projects, m.cursor, m.cursor+1) // <- possible replacement, same stuff more readable
				//1 thing to note is that slices can create Memory Leaks if they lead to pointers. We could only remove pointer from array without ever freeing the thing it points to
				if m.cursor >= len(m.projects) && m.cursor > 0 {
					m.cursor--
				}
				saveConfig(m.projects)
			}
		case "s":
			if len(m.projects) > 0 {
				m.projects[m.cursor].Starred = !m.projects[m.cursor].Starred
				m.sortProjects()
				saveConfig(m.projects)
			}
		case "enter":
			if len(m.projects) > 0 {
				m.projects[m.cursor].LastOpened = time.Now()
				m.selectedPath = m.projects[m.cursor].Path
				if m.selectedPath != "" {
					//Could do save outside and sync them but not needed prolly
					//Honestly this is kinda problematic as it will only save config once emacs is closed
					return m, func() tea.Msg {
						runDailyWorkflow(m.selectedPath, &m)
						return DaemonReadyMsg{}
					}
				}
				m.sortProjects()
				saveConfig(m.projects)
				// return m, tea.Quit
			}
		//For testing functions
		case "t":
			return m, func() tea.Msg {
				return StartDaemonMsg{}
			}
		}
	}
	return m, cmd
}

// --- View (Styled) ---
func (m model) View() string {
	var s string

	// 1. Title
	s += titleStyle.Render("GODAY") + "\n\n"

	// 2. Input Mode
	if m.addingNew {
		s += "Enter path to new project:\n"
		s += m.textInput.View() + "\n\n"
		s += helpStyle.Render("(esc to cancel • enter to save)")
		return docStyle.Render(s)
	}

	// 3. Empty State
	if len(m.projects) == 0 {
		s += lipgloss.NewStyle().Foreground(subtle).Render("No projects found.") + "\n\n"
		s += helpStyle.Render("Press 'a' to add a project.")
		return docStyle.Render(s)
	}

	// 4. List Projects
	for i, p := range m.projects {
		// Determine icons and content
		starIcon := " "
		if p.Starred {
			starIcon = starStyle.Render("★")
		}

		// Base strings
		nameStr := fmt.Sprintf("%s", p.Name)
		// nameStr := fmt.Sprintf("%s %s", starIcon, p.Name)
		pathStr := fmt.Sprintf("  %s", p.Path)

		// Styling selection
		var row string
		if m.cursor == i {
			// Apply selected style
			nameRendered := selectedItemStyle.Render(nameStr)
			pathRendered := pathStyle.Render(pathStr)
			row = lipgloss.JoinHorizontal(lipgloss.Left, starIcon, nameRendered, pathRendered)
		} else {
			// Apply unselected style
			nameRendered := unselectedItemStyle.Render(nameStr)
			// We hide path for unselected items to keep UI clean,
			// or we can show it very dimly. Let's show it dimly.
			pathRendered := pathStyle.Copy().Faint(true).Render(pathStr)
			row = lipgloss.JoinHorizontal(lipgloss.Left, starIcon, nameRendered, pathRendered)
		}

		s += row + "\n"
	}

	s += "\n\n"

	// 5. Daemon status
	daemonInfo := normalStyle.Render("DAEMON status: ")
	//This may be a bit more taxing on perfomancec
	daemonStatus := failureStyle.Render("OFFLINE")
	if m.daemonSpunUp {
		daemonStatus = successStyle.Render("ONLINE")
	}
	s += lipgloss.JoinHorizontal(lipgloss.Left, daemonInfo, daemonStatus)

	// 6. Help Footer
	helpStr := "a: add • s: star • d: delete • enter: open • q: quit"
	s += helpStyle.Render(helpStr)

	return docStyle.Render(s)
}

func main() {
	//Run deamon in the background

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())

	_, err := p.Run()
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func (m *model) sortProjects() {
	sort.Slice(m.projects, func(i, j int) bool {
		if m.projects[i].Starred != m.projects[j].Starred {
			return m.projects[i].Starred
		}
		return m.projects[i].LastOpened.After(m.projects[j].LastOpened)
	})
	// Reset cursor if out of bounds after a delete/sort
	// Dunno if it sohuld be last viable positon
	if m.cursor >= len(m.projects) {
		m.cursor = 0
	}
}

func (m *model) addProject(pathStr string) {
	absPath, err := filepath.Abs(pathStr)
	if err != nil {
		return
	}
	name := filepath.Base(absPath)

	newProj := Project{
		Name:       name,
		Path:       absPath,
		Starred:    false,
		LastOpened: time.Now(),
	}
	m.projects = append(m.projects, newProj)
	m.sortProjects()
	saveConfig(m.projects)
}

// --- Persistence ---
func getConfigPath() string {
	usr, _ := user.Current()
	configDir := filepath.Join(usr.HomeDir, ".config", ConfigDirName)
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		_ = os.MkdirAll(configDir, 0755)
	}
	return filepath.Join(configDir, ConfigFileName)
}

func loadConfig() []Project {
	path := getConfigPath()
	file, err := os.ReadFile(path)
	if err != nil {
		return []Project{}
	}
	var c []Project
	json.Unmarshal(file, &c)
	sort.Slice(c, func(i, j int) bool {
		if c[i].Starred != c[j].Starred {
			return c[i].Starred
		}
		return c[i].LastOpened.After(c[j].LastOpened)
	})
	return c
}

func saveConfig(c []Project) {
	path := getConfigPath()
	data, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(path, data, 0644)
}

// --- File Operations ---
func runDailyWorkflow(projectPath string, m *model) {
	//Is this notation better or is the seperate if and os.stat better?
	//Overall this seems to be a good way to check for errors. So Function does a check -> Do something if it exists (err == nil)
	// OLD -> if _, err := os.Stat(projectPath); os.IsNotExist(err) {
	if _, err := os.Stat(projectPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Project path does not exist: %s\n", projectPath)
		return
	}
	now := time.Now()
	todayFilename := now.Format(DateFormat) + FileExtension
	//This is taking filename from our "standard name"
	//I'd like to make it so that It will actually either know which file is the last one or
	//Use a sorting algo by the date to have them sorted and take the first one
	//If I am already storing the starred projects I may as well store there the last file created
	var yesterdayPath string
	if len(m.projects[m.cursor].LastFileCreated) > 0 {
		yesterdayPath = m.projects[m.cursor].LastFileCreated
	} else {
		yesterdayFilename := now.AddDate(0, 0, -1).Format(DateFormat) + FileExtension
		yesterdayPath = filepath.Join(projectPath, yesterdayFilename)
	}

	todayPath := filepath.Join(projectPath, todayFilename)

	createDailyNote(todayPath, yesterdayPath, m)
	//This has 1 fault, if the list positions change
	emacsHasOpened := openEmacs(m.projects[m.cursor].LastFileCreated, m.projects[m.cursor].PreviousFileCreated)
	for i := 0; !emacsHasOpened && i < 4; i++ {
		time.Sleep(time.Second * DaemonOpenRetryTime)
		emacsHasOpened = openEmacs(m.projects[m.cursor].LastFileCreated, m.projects[m.cursor].PreviousFileCreated)
	}
}

// Could return true if new file was created -> then saveConfig only when was created but need to update LastOpened anyway so left it for now
func createDailyNote(todayFile, yesterdayFile string, m *model) {
	var contentToMigrate []string
	if _, err := os.Stat(yesterdayFile); err == nil {
		contentToMigrate = extractSection(yesterdayFile, SourceHeader)
	}

	tdFile, err := os.Stat(todayFile)
	if err == nil && tdFile != nil {
		//Here we will do things on an already created file, for now nothing
		return
	}
	f, err := os.Create(todayFile)
	if err != nil {
		return
	}

	writer := bufio.NewWriter(f)
	_, _ = writer.WriteString(fmt.Sprintf("#+TITLE: Daily Note %s\n\n", time.Now().Format(DateFormat)))
	_, _ = writer.WriteString(DestinationHeader + "\n")
	if len(contentToMigrate) > 0 {
		for _, line := range contentToMigrate {
			_, _ = writer.WriteString(line + "\n")
		}
	} else {
		_, _ = writer.WriteString("  [ ] \n")
	}
	_, _ = writer.WriteString("\n" + SourceHeader + "\n")
	_ = writer.Flush()

	//Instead of defer I put f.Close() at the end for clarity of reading
	//The mini problem with defers in go is that you can't defer assignments
	//And it makes it so that the code is not read from Top to Bottom
	f.Close()
	m.projects[m.cursor].PreviousFileCreated = m.projects[m.cursor].LastFileCreated
	m.projects[m.cursor].LastFileCreated = todayFile
	saveConfig(m.projects)
}

func extractSection(filename, targetHeader string) []string {
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()

	var lines []string
	capture := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == targetHeader {
			capture = true
			continue
		}
		if capture {
			if strings.HasPrefix(line, "* ") {
				break
			}
			lines = append(lines, line)
		}
	}
	return lines
}

func openEmacs(currentFilePath, previousFilePath string) bool {
	cmd := exec.Command("emacsclient", "-r", currentFilePath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	go func() {
		time.Sleep(time.Second / 10) // to ensure it runs after blocking cmd.Run() below

		cmd3 := exec.Command("emacsclient")
		splitOpenEmacsCmd := fmt.Sprintf(`emacsclient -n -r --eval "(progn (delete-other-windows) (find-file \"%s\") (split-window-right) (find-file \"%s\"))"`, filepath.ToSlash(previousFilePath), filepath.ToSlash(currentFilePath))
		cmd3.SysProcAttr = &syscall.SysProcAttr{
			CmdLine: splitOpenEmacsCmd,
		}
		err := cmd3.Run()
		if err != nil {
			return
		}
	}()
	err := cmd.Run()
	if err != nil {
		return false
	}
	return true
}

// Tried reading console output from cmd.Stdout by passing a buffer but failed at it
// For now I'll just have a set amount of time to wait for daemon to run
func runEmacsDaemon(returnChan chan<- string) {
	file, err := os.Create("C:\\Users\\Ramand\\Desktop\\goTerminal\\firstApp\\output.log")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	cmd := exec.Command("emacs", "--daemon")
	stderr, _ := cmd.StderrPipe()

	cmd.Start()

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, DoomLoadedMsg) {
			returnChan <- SuccessMsg
		}
	}

	cmd.Wait()
}

// func runEmacsDaemon() {
// 	cmd := exec.Command("emacs", "--daemon")
// 	cmd.Stdin = nil
// 	cmd.Stdout = nil
// 	cmd.Stderr = nil
// 	cmd.Run()
// }

func cmdSetupAndRun(cmd *exec.Cmd) {
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Run()
}
