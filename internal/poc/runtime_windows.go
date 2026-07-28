//go:build windows

package poc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/pocassets"

	SunnyNet "github.com/qtgolang/SunnyNet/SunnyNet"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
func (systemClock) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (systemClock) After(duration time.Duration) <-chan time.Time { return time.After(duration) }

type runtimeBridgeCloser struct {
	once     sync.Once
	server   *http.Server
	listener net.Listener
	bridge   Bridge
	err      error
}

func (c *runtimeBridgeCloser) Close() error {
	c.once.Do(func() {
		var errs []error
		if c.bridge != nil {
			errs = append(errs, c.bridge.Close())
		}
		if c.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			errs = append(errs, c.server.Shutdown(ctx))
			cancel()
		} else if c.listener != nil {
			errs = append(errs, c.listener.Close())
		}
		c.err = errors.Join(errs...)
	})
	return c.err
}

func newPlatformRuntime(input io.Reader, output io.Writer) (*Runtime, error) {
	runner := ExecCommandRunner{}
	controls := make(chan OperatorCommand, 4)
	scanner := bufio.NewScanner(input)
	options := DefaultOptions()
	_, bridgePortText, err := net.SplitHostPort(options.BridgeAddress)
	if err != nil {
		return nil, errors.New("invalid approved bridge address")
	}
	bridgePort, err := strconv.Atoi(bridgePortText)
	if err != nil {
		return nil, errors.New("invalid approved bridge port")
	}
	deps := RuntimeDeps{
		Preflight: func(ctx context.Context, options Options) (PreflightReport, error) {
			return NewPreflight(runner, NewWindowsVMDetector(runner)).Run(ctx, options)
		},
		CreateCA: func(jobID string) (*JobCA, error) { return GenerateJobCA(jobID, time.Now().UTC()) },
		Approve: func(ctx context.Context, summary ChangeSummary) error {
			_, _ = fmt.Fprintf(output, "Planned changes: certificate=%s proxy=%s bridge=%s process=%s\nType APPLY to continue: ", summary.CertificateScope, summary.ProxyAddress, summary.BridgeAddress, summary.ProcessName)
			line := make(chan string, 1)
			go func() {
				if scanner.Scan() {
					line <- scanner.Text()
					return
				}
				close(line)
			}()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case value, ok := <-line:
				if !ok || value != "APPLY" {
					return errors.New("exact APPLY confirmation is required")
				}
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if scanner.Err() != nil {
				return errors.New("exact APPLY confirmation is required")
			}
			go readOperatorCommands(scanner, controls)
			return nil
		},
		CertStore: newCertificateStore(runner),
		BridgeStart: func(ctx context.Context, address, token string) (Bridge, io.Closer, error) {
			return startRuntimeBridge(ctx, address, token)
		},
		ProxyFactory: func(_ context.Context, ca *JobCA, address, token string) (RuntimeProxy, error) {
			sunny, err := ca.NewSunny()
			if err != nil {
				return nil, err
			}
			proxy := NewProxy(newRealSunnyCore(sunny), address)
			proxy.ConfigureInjector(pocassets.APIClientJS, BridgeConfig{Port: bridgePort, Token: token})
			return proxy, nil
		},
		Collect: func(ctx context.Context, bridge Bridge, store *Store, options Options) (Dataset, Validation, error) {
			return collectRestrictedPOC(ctx, bridge, store, options, controls)
		},
		StoreFactory: func(options Options, jobID string) (*Store, error) {
			repoRoot, err := existingCanonicalDirectory(".")
			if err != nil {
				return nil, errors.New("run POC from repository root")
			}
			return NewStore(StoreOptions{
				RepoRoot: repoRoot, DataRoot: options.DataRoot, SecretsRoot: options.SecretsRoot,
				RuntimeRoot: options.RuntimeRoot, BuildRoot: options.BuildRoot, VarRoot: "var", JobID: jobID,
			})
		},
		LoggerFactory: func(store *Store) (SafeLogger, error) {
			return NewFileSafeLogger(filepath.Join(store.JobDir(), "run.log"))
		},
	}
	return NewRuntime(deps), nil
}

func readOperatorCommands(scanner *bufio.Scanner, controls chan<- OperatorCommand) {
	defer close(controls)
	for scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "EXTEND":
			controls <- OperatorExtend
		case "CANCEL":
			controls <- OperatorCancel
		}
	}
}

func startRuntimeBridge(_ context.Context, address, token string) (Bridge, io.Closer, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, nil, errors.New("bridge listener must be loopback")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, nil, errors.New("start bridge listener")
	}
	bridge := NewBridgeServer(token, DiscardLogger{})
	server := &http.Server{Handler: bridge.Handler(), ReadHeaderTimeout: 5 * time.Second}
	closer := &runtimeBridgeCloser{server: server, listener: listener, bridge: bridge}
	go func() { _ = server.Serve(listener) }()
	return bridge, closer, nil
}

func collectRestrictedPOC(ctx context.Context, bridge Bridge, store *Store, options Options, controls <-chan OperatorCommand) (Dataset, Validation, error) {
	clock := systemClock{}
	waiter := NewWaitController(clock, options.HumanWait, controls)
	if result := waitForBridge(ctx, bridge, waiter, WaitLogin, 0, []string{"finderSearch", "finderGetCommentList"}); result != WaitResolved {
		job, capability, coverage, reasons := EvaluateOutcome(OutcomeInput{HumanTimedOut: result == WaitTimedOut, HumanCancelled: result == WaitCancelled})
		return Dataset{Job: Job{Status: job, CapabilityStatus: capability, CoverageStatus: coverage}}, Validation{CapabilityStatus: capability, CoverageStatus: coverage, ReasonCodes: reasons}, HumanWaitError{Result: result}
	}

	recorder, rawCleanup, err := runtimeEvidenceRecorder(store, options.AllowEncryptedRaw)
	if err != nil {
		return Dataset{}, Validation{}, err
	}
	defer rawCleanup()
	collector := NewCollector(bridge, recorder, store, clock)
	collector.ConfigureHumanWait(waiter, func(_ WaitReason, _ int) <-chan struct{} {
		ready := make(chan struct{})
		go func() {
			if bridge.WaitReady(ctx, []string{"finderGetCommentList"}) == nil {
				close(ready)
			}
		}()
		return ready
	})
	works, coverage, collectErr := collector.CollectWorks(ctx, options)
	comments := make([]Comment, 0)
	completedWorks := 0
	for index := range works {
		if collectErr != nil {
			break
		}
		collected, summary, err := collector.CollectComments(ctx, options, works[index])
		comments = append(comments, collected...)
		works[index].TopLevelCommentCount = summary.TopLevel
		works[index].ReplyCount = summary.Replies
		works[index].Truncation.Truncated = summary.Truncated
		works[index].Truncation.Reasons = append([]string(nil), summary.Reasons...)
		if err != nil {
			works[index].CollectionStatus = "partial"
			collectErr = err
			break
		}
		works[index].CollectionStatus = "completed"
		completedWorks++
	}
	input := outcomeFromCollection(coverage, works, completedWorks, comments, collectErr)
	jobStatus, capability, finalCoverage, reasons := EvaluateOutcome(input)
	completedAt := time.Now().UTC()
	dataset := Dataset{SchemaVersion: SchemaVersion, Job: Job{
		Keyword: options.Keyword, Status: jobStatus, CapabilityStatus: capability, CoverageStatus: finalCoverage,
		StartedAt: completedAt, CompletedAt: &completedAt, Limits: options.Limits,
	}, Works: works, Comments: comments}
	validation := Validation{CapabilityStatus: capability, CoverageStatus: finalCoverage, Fields: validationFields(comments), ReasonCodes: reasons}
	return dataset, validation, collectErr
}

func waitForBridge(ctx context.Context, bridge Bridge, waiter *WaitController, reason WaitReason, rank int, methods []string) WaitResult {
	ready := make(chan struct{})
	go func() {
		if bridge.WaitReady(ctx, methods) == nil {
			close(ready)
		}
	}()
	return waiter.Wait(ctx, reason, rank, ready)
}

func runtimeEvidenceRecorder(store *Store, enabled bool) (*EvidenceRecorder, func(), error) {
	if !enabled {
		return NewEvidenceRecorder(nil), func() {}, nil
	}
	directory := filepath.Join(store.SecretsDir(), "raw-evidence")
	if err := os.Mkdir(directory, 0o700); err != nil {
		return nil, nil, errors.New("create encrypted raw directory")
	}
	if err := secureDirectory(directory); err != nil {
		return nil, nil, err
	}
	rawStore, err := NewEncryptedRawStore(directory, true)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = rawStore.Destroy()
		_ = os.Remove(directory)
	}
	return NewEvidenceRecorder(rawStore), cleanup, nil
}

func outcomeFromCollection(coverage CoverageStatus, works []Work, completed int, comments []Comment, collectErr error) OutcomeInput {
	input := OutcomeInput{SearchComplete: coverage != CoverageIncomplete, SourceExhausted: coverage == CoverageSourceExhausted, ValidWorks: len(works), CompletedWorks: completed}
	for _, comment := range comments {
		if comment.Level == 1 {
			input.TopLevelComments++
		} else if comment.Level == 2 {
			input.Replies++
		}
	}
	if collectErr != nil {
		var human HumanWaitError
		if errors.As(collectErr, &human) {
			input.HumanTimedOut = human.Result == WaitTimedOut
			input.HumanCancelled = human.Result == WaitCancelled
		}
		input.SchemaFailed = ClassifyError(collectErr) == ErrorStructure
	}
	input.RequiredFieldStatuses = requiredFieldStatuses(comments)
	return input
}

func requiredFieldStatuses(comments []Comment) map[string]FieldStatus {
	statuses := map[string]FieldStatus{
		"comment_id": FieldPresent, "content": FieldPresent, "account": FieldPresent,
		"time": FieldPresent, "ip_location": FieldPresent, "media_type": FieldPresent,
	}
	for _, comment := range comments {
		if comment.CommentID == nil {
			statuses["comment_id"] = FieldMissingInSource
		}
		if comment.Content.Text == nil {
			statuses["content"] = FieldMissingInSource
		}
		if comment.Account.AccountID == nil && comment.Account.DisplayName == nil {
			statuses["account"] = FieldMissingInSource
		}
		if comment.CreatedAt.Raw == nil {
			statuses["time"] = FieldMissingInSource
		}
		if comment.IPLocation.Label == nil {
			statuses["ip_location"] = FieldMissingInSource
		}
		if comment.Content.MediaType.RawCode == nil {
			statuses["media_type"] = FieldMissingInSource
		}
	}
	return statuses
}

func validationFields(comments []Comment) []FieldResult {
	statuses := requiredFieldStatuses(comments)
	fields := make([]FieldResult, 0, len(statuses))
	for _, name := range []string{"comment_id", "content", "account", "time", "ip_location", "media_type"} {
		fields = append(fields, fieldResult("comments[]."+name, statuses[name]))
	}
	return fields
}

func runPlatformPreflight(ctx context.Context, options Options) (PreflightReport, error) {
	runner := ExecCommandRunner{}
	return NewPreflight(runner, NewWindowsVMDetector(runner)).Run(ctx, options)
}

func runPlatformCleanup(ctx context.Context, options Options, jobID string) error {
	repoRoot, err := existingCanonicalDirectory(".")
	if err != nil {
		return errors.New("run cleanup from repository root")
	}
	runtimeRoot, err := resolveStorePath(repoRoot, options.RuntimeRoot)
	if err != nil {
		return err
	}
	runtimeDir := filepath.Join(runtimeRoot, jobID)
	if !pathWithin(runtimeRoot, runtimeDir) {
		return errors.New("cleanup runtime directory escapes approved root")
	}
	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		return nil
	}
	if err := ensurePathComponentsSafe(repoRoot, runtimeDir, false); err != nil {
		return errors.New("cleanup runtime directory failed validation")
	}
	for _, address := range []string{options.ProxyAddress, options.BridgeAddress} {
		listener, listenErr := net.Listen("tcp", address)
		if listenErr != nil {
			return errors.New("POC runtime still appears active")
		}
		_ = listener.Close()
	}
	store, err := NewStore(StoreOptions{
		RepoRoot: repoRoot, DataRoot: options.DataRoot, SecretsRoot: options.SecretsRoot,
		RuntimeRoot: options.RuntimeRoot, BuildRoot: options.BuildRoot, VarRoot: "var", JobID: jobID,
	})
	if err != nil {
		return err
	}
	state, err := store.LoadRuntimeState()
	if err != nil {
		return err
	}
	runner := ExecCommandRunner{}
	stopDriver := func(unregister bool) error {
		sunny := SunnyNet.NewSunny()
		core := newRealSunnyCore(sunny)
		deleteErr := core.DeleteProcessName(weChatProcessName)
		stopErr := core.StopProcess(unregister)
		return errors.Join(deleteErr, stopErr)
	}
	return cleanupPersistedRuntime(ctx, store, state, newCertificateStore(runner), stopDriver, nil, time.Now().UTC())
}
