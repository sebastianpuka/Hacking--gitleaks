package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type ReportElem struct {
	Lines   []string `json:"lines"`
	Branch  string   `json:"branch"`
	CommitA string   `json:"commitA"`
	CommitB string   `json:"commitB"`
}

type Repo struct {
	url  string
	name string
	path string
}

type MemoMessage struct {
	hash    string
	leaks   []string
	commitA string
	commitB string
	branch  string
}

var lock = sync.RWMutex{}

func repoStart(repoUrl string) {
	err := exec.Command("git", "clone", repoUrl).Run()
	if err != nil {
		log.Fatalf("failed to clone repo %v", err)
	}
	repoName := strings.Split(repoUrl, "/")[4]
	if err := os.Chdir(repoName); err != nil {
		log.Fatal(err)
	}

	repo := Repo{repoUrl, repoName, ""}
	report := repo.audit()
	repo.cleanup()

	reportJson, _ := json.MarshalIndent(report, "", "\t")
	err = ioutil.WriteFile(fmt.Sprintf("%s_leaks.json", repo.name), reportJson, 0644)
}

// cleanup changes to app root and recursive rms target repo
func (repo Repo) cleanup() {
	if err := os.Chdir(appRoot); err != nil {
		log.Fatalf("failed cleaning up repo. Does the repo exist? %v", err)
	}
	err := exec.Command("rm", "-rf", repo.name).Run()
	if err != nil {
		log.Fatal(err)
	}
}

// audit parses git branch --all
func (repo Repo) audit() []ReportElem {
	var (
		out     []byte
		err     error
		branch  string
		commits [][]byte
		// leaks   []string
		wg      sync.WaitGroup
		commitA string
		commitB string
	)

	out, err = exec.Command("git", "branch", "--all").Output()
	if err != nil {
		log.Fatalf("error retrieving branches %v\n", err)
	}

	// iterate through branches, git rev-list <branch>
	branches := bytes.Split(out, []byte("\n"))

	messages := make(chan MemoMessage)

	for i, branchB := range branches {
		if i < 2 || i == len(branches)-1 {
			continue
		}
		branch = string(bytes.Trim(branchB, " "))
		cmd := exec.Command("git", "rev-list", branch)

		out, err := cmd.Output()
		if err != nil {
			fmt.Println("skipping branch", branch)
			continue
		}

		// iterate through commits
		commits = bytes.Split(out, []byte("\n"))
		wg.Add(len(commits) - 2)
		for j, currCommit := range commits {
			if j == len(commits)-2 {
				break
			}
			commitA = string(commits[j+1])
			commitB = string(currCommit)

			go func(commitA string, commitB string,
				j int) {
				defer wg.Done()
				var leakPrs bool
				var leaks []string
				fmt.Println(j, branch)

				lock.RLock()
				_, seen := cache[commitA+commitB]
				lock.RUnlock()
				if seen {
					fmt.Println("WE HAVE SEEN THIS")
					return
				}
				if err := os.Chdir(fmt.Sprintf("%s/%s", appRoot, repo.name)); err != nil {
					log.Fatal(err)
				}
				cmd := exec.Command("git", "diff", commitA, commitB)
				out, err := cmd.Output()
				if err != nil {
					return
				}
				lines := checkRegex(string(out))
				if len(lines) == 0 {
					return
				}

				for _, line := range lines {
					leakPrs = checkEntropy(line)
					if leakPrs {
						leaks = append(leaks, line)
					}
				}

				// if len(leaks) != 0 {
				// 	report = append(report, ReportElem{leaks, branch,
				// 		string(commitB), string(commits[j+1])})
				// }
				messages <- MemoMessage{commitA + commitB, leaks, commitA, commitB, branch}

			}(commitA, commitB, j)
		}

		go func() {
			for memoMsg := range messages {
				fmt.Println(memoMsg)
				lock.Lock()
				cache[memoMsg.hash] = true
				lock.Unlock()
				if len(memoMsg.leaks) != 0 {
					report = append(report, ReportElem{memoMsg.leaks, memoMsg.branch,
						memoMsg.commitA, memoMsg.commitB})
				}
			}
		}()
		wg.Wait()
	}

	return report
}
