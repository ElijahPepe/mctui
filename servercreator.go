package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pbnjay/memory"
)

var (
	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))

	selectorTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FAFAFA")).
				Background(lipgloss.Color("#7D56F4")).
				Bold(true).
				Padding(0, 1)

	selectorNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#DDDDDD"}).
				Padding(0, 0, 0, 2)

	selectorSelectedStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(lipgloss.AdaptiveColor{Light: "#9F72FF", Dark: "#AD58B4"}).
				Foreground(lipgloss.AdaptiveColor{Light: "#9A4AFF", Dark: "#EE6FF8"}).
				Bold(true).
				Padding(0, 0, 0, 1)

	selectorPaginationStyle = list.DefaultStyles().PaginationStyle.PaddingLeft(4)

	selectorHelpStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6F6C6C", Dark: "#7A7A7A"})
)

type selectorItem struct {
	ct    string
	title string
}

func (cti selectorItem) FilterValue() string { return cti.title }

type selectorDelegate struct{}

func (d selectorDelegate) Height() int                             { return 1 }
func (d selectorDelegate) Spacing() int                            { return 0 }
func (d selectorDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d selectorDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(selectorItem)
	if !ok {
		return
	}
	str := fmt.Sprintf("%d. %s", index+1, i.title)
	if index == m.Index() {
		_, _ = fmt.Fprintf(w, selectorSelectedStyle.Render(str))
	} else {
		_, _ = fmt.Fprintf(w, selectorNormalStyle.Render(str))
	}

}

type selectorModel struct {
	list    list.Model
	choice  string
	spinner spinner.Model
}

type Version struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []struct {
		ID          string    `json:"id"`
		Type        string    `json:"type"`
		URL         string    `json:"url"`
		Time        time.Time `json:"time"`
		ReleaseTime time.Time `json:"releaseTime"`
	} `json:"versions"`
}

type URLs struct {
	Arguments struct {
		Game []interface{} `json:"game"`
		Jvm  []interface{} `json:"jvm"`
	} `json:"arguments"`
	AssetIndex struct {
		ID        string `json:"id"`
		Sha1      string `json:"sha1"`
		Size      int    `json:"size"`
		TotalSize int    `json:"totalSize"`
		URL       string `json:"url"`
	} `json:"assetIndex"`
	Assets          string `json:"assets"`
	ComplianceLevel int    `json:"complianceLevel"`
	Downloads       struct {
		Client struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			URL  string `json:"url"`
		} `json:"client"`
		ClientMappings struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			URL  string `json:"url"`
		} `json:"client_mappings"`
		Server struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			URL  string `json:"url"`
		} `json:"server"`
		ServerMappings struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			URL  string `json:"url"`
		} `json:"server_mappings"`
	} `json:"downloads"`
	ID          string `json:"id"`
	JavaVersion struct {
		Component    string `json:"component"`
		MajorVersion int    `json:"majorVersion"`
	} `json:"javaVersion"`
	Libraries []struct {
		Downloads struct {
			Artifact struct {
				Path string `json:"path"`
				Sha1 string `json:"sha1"`
				Size int    `json:"size"`
				URL  string `json:"url"`
			} `json:"artifact"`
		} `json:"downloads"`
		Name  string `json:"name"`
		Rules []struct {
			Action string `json:"action"`
			Os     struct {
				Name string `json:"name"`
			} `json:"os"`
		} `json:"rules,omitempty"`
	} `json:"libraries"`
	Logging struct {
		Client struct {
			Argument string `json:"argument"`
			File     struct {
				ID   string `json:"id"`
				Sha1 string `json:"sha1"`
				Size int    `json:"size"`
				URL  string `json:"url"`
			} `json:"file"`
			Type string `json:"type"`
		} `json:"client"`
	} `json:"logging"`
	MainClass              string    `json:"mainClass"`
	MinimumLauncherVersion int       `json:"minimumLauncherVersion"`
	ReleaseTime            time.Time `json:"releaseTime"`
	Time                   time.Time `json:"time"`
	Type                   string    `json:"type"`
}

func getVersions() Version {
	res, err := http.Get("https://launchermeta.mojang.com/mc/game/version_manifest.json")
	if err != nil {
		fmt.Println(err)
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
	}

	var result Version
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Println(err)
	}

	return result
}

func (m selectorModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func getURLs(url string) URLs {
	res, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
	}

	var result URLs
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Println(err)
	}

	return result
}

func downloadFile(location string, url string, wg *sync.WaitGroup, m selectorModel) {
	defer wg.Done()
	out, err := os.Create(location)
	if err != nil {
		fmt.Println(err)
	}
	defer out.Close()
	res, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("\n\n  %sDownloading server.jar\n\n", m.spinner.View())
	defer res.Body.Close()
	io.Copy(out, res.Body)
}

func createFlags() string {
	totalMemory := memory.TotalMemory()
	if totalMemory >= 12000000000 {
		return "-Xms10G -Xmx10G -XX:+UseG1GC -XX:+ParallelRefProcEnabled -XX:MaxGCPauseMillis=200 -XX:+UnlockExperimentalVMOptions -XX:+DisableExplicitGC -XX:+AlwaysPreTouch -XX:G1NewSizePercent=40 -XX:G1MaxNewSizePercent=50 -XX:G1HeapRegionSize=16M -XX:G1ReservePercent=15 -XX:G1HeapWastePercent=5 -XX:G1MixedGCCountTarget=4 -XX:InitiatingHeapOccupancyPercent=20 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:SurvivorRatio=32 -XX:+PerfDisableSharedMem -XX:MaxTenuringThreshold=1 -Dusing.aikars.flags=https://mcflags.emc.gs -Daikars.new.flags=true -jar"
	} else {
		return "-Xms10G -Xmx10G -XX:+UseG1GC -XX:+ParallelRefProcEnabled -XX:MaxGCPauseMillis=200 -XX:+UnlockExperimentalVMOptions -XX:+DisableExplicitGC -XX:+AlwaysPreTouch -XX:G1NewSizePercent=30 -XX:G1MaxNewSizePercent=40 -XX:G1HeapRegionSize=8M -XX:G1ReservePercent=20 -XX:G1HeapWastePercent=5 -XX:G1MixedGCCountTarget=4 -XX:InitiatingHeapOccupancyPercent=15 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:SurvivorRatio=32 -XX:+PerfDisableSharedMem -XX:MaxTenuringThreshold=1 -Dusing.aikars.flags=https://mcflags.emc.gs -Daikars.new.flags=true -jar"
	}
}

func acceptEULA() {
	input, err := ioutil.ReadFile("server/eula.txt")
	if err != nil {
		fmt.Println(err)
	}
	lines := strings.Split(string(input), "\n")
	lines[2] = "eula=true"
	output := strings.Join(lines, "\n")
	err = ioutil.WriteFile("server/eula.txt", []byte(output), 0777)
	if err != nil {
		fmt.Println(err)
	}
}

func createServer(choice string, m selectorModel) {
	os.Mkdir("server", 0777)
	var wg sync.WaitGroup
	wg.Add(1)
	downloadFile("server/server.jar", getURLs(choice).Downloads.Server.URL, &wg, m)

	checkmark := lipgloss.NewStyle().SetString("✓").
		PaddingRight(1).
		Foreground(lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}).
		String()
	fmt.Printf("\r  %sDownloaded server.jar\n\n", checkmark)

	cmd := exec.Command("cmd", "/C", "java", createFlags(), "server/server.jar", "--nogui")
	err := cmd.Run()
	aikarFlags := true
	if err != nil {
		// Let's try this again, without Aikar's flags.
		exec.Command("cmd", "/C", "cd", "server", "&&", "java", "-jar", "server.jar", "--nogui").Run()
		aikarFlags = false
	}

	acceptEULA()
	fmt.Printf("\r  %sAccepted EULA automatically.\n\n", checkmark)

	var output string
	if !aikarFlags {
		output = "java -jar server.jar --nogui"
	} else {
		output = "java " + createFlags() + " server.jar " + "--nogui"
	}
	err = ioutil.WriteFile("server/run.bat", []byte(output), 0777)
	if err != nil {
		fmt.Println(err)
	}
}

func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "esc":
			return m, tea.Quit

		case "enter":
			m.choice = m.list.SelectedItem().(selectorItem).ct
			createServer(m.choice, m)
			return m, tea.Quit

		default:
			m.spinner.Update(msg)
			if !m.list.SettingFilter() && (keypress == "q" || keypress == "esc") {
				return m, tea.Quit
			}

			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

	default:
		return m, nil
	}
}

func (m selectorModel) View() string {
	if m.choice != "" {
		return m.choice
	}
	return "\n" + m.list.View()
}

func newSelectorModel() selectorModel {
	listItems := makeSelectorItems()
	entireList := make([]list.Item, 0, len(listItems))
	for _, item := range listItems {
		entireList = append(entireList, item)
	}
	l := list.New(entireList, selectorDelegate{}, 20, 12)

	l.Title = "Server version"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = selectorTitleStyle
	l.Styles.PaginationStyle = selectorPaginationStyle
	h := help.NewModel()
	h.Styles.ShortDesc = selectorHelpStyle
	h.Styles.ShortSeparator = selectorHelpStyle
	h.Styles.ShortKey = selectorHelpStyle
	l.Help = h

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return selectorModel{spinner: s, list: l}
}

func makeSelectorItems() []selectorItem {
	list := getVersions()
	var versionList []string
	var urlList []string
	for _, version := range list.Versions {
		r, _ := regexp.Compile("0*[1-9][0-9]*.[1-9][0-9]")
		if strings.Contains(version.ID, "-") || strings.Contains(version.ID, "w") || !r.MatchString(version.ID) {
			continue
		}
		if !r.MatchString(version.ID) {
			continue
		}
		id := version.ID
		url := version.URL
		versionList = append(versionList, id)
		urlList = append(urlList, url)
	}

	selectorItems := make([]selectorItem, len(versionList))

	for i, version := range versionList {
		selectorItems[i] = selectorItem{urlList[i], version}
	}
	return selectorItems
}

func main() {
	makeSelectorItems()
	p := tea.NewProgram(newSelectorModel())
	if err := p.Start(); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
}
