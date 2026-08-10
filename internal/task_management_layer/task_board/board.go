// Package taskboard runs the Publish-Lease protocol for task distribution.
// It plays both roles:
//
//	Employer (plan owner): on TaskReady, broadcast a TaskAd, collect
//	  CapabilityCV bids from peers (plus its own self-bid), and after a short
//	  window assign the best bid — locally, or to the winning peer via
//	  postman.SendEvent.
//	Employee (worker): on a peer's TaskAd, evaluate local capability and bid
//	  with a CV; on a remote TaskAssign, re-publish TaskAssigned locally so
//	  the executor runs.
//
// Results flow back through the postman's TaskTraced → Owner routing, so the
// employer's commander sees remote results exactly like local ones.
package taskboard

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	APICapability "github.com/cpmores/lucinda/api/v1/domain/capability"
	APINode "github.com/cpmores/lucinda/api/v1/domain/node"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	taskpostman "github.com/cpmores/lucinda/internal/task_management_layer/postman"
	eventbus "github.com/cpmores/lucinda/pkg/infrastructure_layer/eventbus"
	hardwaremonitor "github.com/cpmores/lucinda/pkg/infrastructure_layer/hardware_monitor"
	logger "github.com/cpmores/lucinda/pkg/infrastructure_layer/logger"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	providerctrl "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider"
	transport "github.com/cpmores/lucinda/pkg/infrastructure_layer/transport"
)

// bidWindow is how long the employer waits for peer CVs before assigning.
// The self-bid is always present, so a single node assigns itself after one
// window; peers' bids may outscore it.
const bidWindow = 150 * time.Millisecond

// reasonBidWindow is the window for reason tasks. It is kept short enough
// not to stall the ReAct loop when the local node serves the model, yet long
// enough for a remote bid when it does not.
const reasonBidWindow = 300 * time.Millisecond

// TaskBoard is the interface for the board module.
type TaskBoard interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error
}

// maxAdRetries bounds how many times an advertisement is re-issued when no
// bid qualifies, before the task is given up (which fails the plan).
const maxAdRetries = 3

// firstCandidate bounds the number of candidates to consider for assignment.
// If first one failed, then get the second, etc.
const firstCandidate = 3

// hiringState tracks one of my tasks from advertisement to assignment.
type hiringState struct {
	task       *APITask.Task
	bids       []APICapability.CapabilityCV
	bestScores []APICapability.BestScore
	candidate  int
	assigned   bool
	retries    int
}

type board struct {
	mm  modulemanager.ModuleManager
	eb  eventbus.EventBus
	tp  transport.Transport
	pm  taskpostman.TaskPostman
	pc  providerctrl.ProviderController
	hm  hardwaremonitor.HardwareMonitor // optional
	log *logger.Logger

	ctx    context.Context
	mu     sync.Mutex
	myAds  map[APITask.TaskID]*hiringState
	cancel context.CancelFunc
}

// NewTaskBoard creates a board. Deps are resolved via DependsEnable
// (HardwareMonitor is optional).
func NewTaskBoard(log *logger.Logger) TaskBoard {
	if log == nil {
		log = logger.Discard()
	}
	log.Info("created")
	return &board{
		log:   log,
		myAds: make(map[APITask.TaskID]*hiringState),
	}
}

// ── Lifecycle ──────────────────────────────────────────────────────────

func (b *board) Start(ctx context.Context) error {
	ctx, b.cancel = context.WithCancel(ctx)
	b.ctx = ctx

	watch := func(topic APIEvent.EventType, handler func(any)) {
		ch := b.eb.Subscribe(topic, 64)
		go func() {
			defer b.eb.UnSubscribe(topic, ch)
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-ch:
					if !ok {
						return
					}
					handler(ev.Data)
				}
			}
		}()
	}

	watch(APIEvent.TaskReady, func(data any) {
		if task, ok := data.(*APITask.Task); ok {
			b.onReady(task)
		}
	})
	watch(APIEvent.TaskAdReceived, func(data any) {
		if msg, ok := data.(APITaskmsg.TaskBroadcastMsg); ok {
			b.onAd(&msg)
		}
	})
	watch(APIEvent.TaskCVReceived, func(data any) {
		if msg, ok := data.(APITaskmsg.TaskCVMsg); ok {
			b.onCV(&msg)
		}
	})
	watch(APIEvent.TaskAssign, func(data any) {
		if msg, ok := data.(APITaskmsg.TaskAssignMsg); ok {
			b.onAssign(&msg)
		}
	})
	watch(APIEvent.TaskTraced, b.onTaskTerminated)

	b.log.Info("started", "local", b.tp.ID())
	return nil
}

func (b *board) Stop() error {
	if b.cancel != nil {
		b.cancel()
	}
	b.log.Info("stopped")
	return nil
}

// ── Employer side ──────────────────────────────────────────────────────

// onReady advertises a ready task, self-bids, and schedules the assignment.
func (b *board) onReady(task *APITask.Task) {
	hs := &hiringState{task: task}
	b.mu.Lock()
	b.myAds[task.Meta.ID] = hs
	b.mu.Unlock()
	b.advertise(hs)
}

// advertise self-bids, broadcasts the ad, and schedules the assignment.
func (b *board) advertise(hs *hiringState) {
	// Self-bid so a lone node (or the best local candidate) can execute. A
	// node with no models can't serve an LLM task, so it doesn't self-bid and
	// lets a capable remote node win.
	if cv := b.buildCV(hs.task.Meta.ID); len(cv.Models) > 0 && matchCV(&cv, &hs.task.Spec) >= 0 {
		hs.bids = append(hs.bids, cv)
	}

	ad := APITask.TaskToTaskAd(hs.task)
	_ = b.pm.BroadcastEvent(b.ctx, APIEvent.TaskAdReceived, APITaskmsg.TaskBroadcastMsg{
		TaskID: hs.task.Meta.ID,
		Spec:   ad.Spec,
		Owner:  hs.task.Meta.Owner,
	})
	b.log.Info("advertised", "task", hs.task.Meta.ID, "plan", hs.task.Meta.Owner)

	window := bidWindow
	if hs.task.Spec.Kind == APITask.TaskKindReason {
		window = reasonBidWindow
	}

	go func() {
		time.Sleep(window)
		b.assignBest(hs.task.Meta.ID)
	}()
}

// onCV collects a peer's bid on one of my ads.
func (b *board) onCV(msg *APITaskmsg.TaskCVMsg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	hs := b.myAds[msg.TaskID]
	if hs == nil || hs.assigned {
		return
	}
	if cv := msg.CV; matchCV(&cv, &hs.task.Spec) >= 0 {
		hs.bids = append(hs.bids, cv)
		b.log.Debug("bid collected", "task", msg.TaskID, "peer", cv.PeerID)
	}
}

// assignBest picks the highest-scoring bid after the window and hands the
// task to the winner (local publish or remote unicast). If no bid qualified,
// it re-advertises up to maxAdRetries instead of dropping the task.
func (b *board) assignBest(taskID APITask.TaskID) {
	b.mu.Lock()
	hs := b.myAds[taskID]
	if hs == nil || hs.assigned {
		b.mu.Unlock()
		return
	}
	bests := bestBids(hs.bids, &hs.task.Spec)
	// NOTE: RETRY Done here
	if bests == nil {
		if hs.retries < maxAdRetries {
			hs.retries++
			b.mu.Unlock()
			b.log.Warn("no qualified bid, re-advertising", "task", taskID, "retry", hs.retries)
			b.advertise(hs)
			return
		}
		b.mu.Unlock()
		b.log.Error("no qualified bid for task, giving up", "task", taskID)
		// Fail the plan so it does not hang forever.
		b.failPlan(taskID)
		return
	}

	best := bests[0].Best
	hs.bestScores = bests
	hs.assigned = true
	b.mu.Unlock()

	assign := APITaskmsg.TaskAssignMsg{
		TaskID: taskID,
		Spec:   hs.task.Spec,
		Prompt: hs.task.Spec.Prompt,
		Owner:  hs.task.Meta.Owner,
		PlanID: hs.task.TaskPlan.ID,
	}
	if best.PeerID == string(b.tp.ID()) {
		_ = b.eb.Publish(APIEvent.TaskAssigned, APIEvent.NewEvent(APIEvent.TaskAssigned, assign))
		b.log.Info("self-assigned", "task", taskID)
		return
	}
	if err := b.pm.SendEvent(b.ctx, APINode.NodeID(best.PeerID), APIEvent.TaskAssign, assign); err != nil {
		b.log.Error("assign failed", "task", taskID, "peer", best.PeerID, "err", err)
	}
	b.log.Info("assigned to peer", "task", taskID, "peer", best.PeerID)
}

// failPlan terminates the plan when a task can't be assigned after all
// retries: it emits a TaskTraced Failed, which the commander turns into a
// PlanError, so the plan fails cleanly instead of hanging.
func (b *board) failPlan(taskID APITask.TaskID) {
	b.mu.Lock()
	hs := b.myAds[taskID]
	delete(b.myAds, taskID)
	b.mu.Unlock()
	if hs == nil {
		return
	}
	msg := APITaskmsg.TaskTracedMsg{
		TaskID: taskID,
		PlanID: hs.task.TaskPlan.ID,
		State:  APITask.StateFailed,
		Owner:  hs.task.Meta.Owner,
	}
	_ = b.eb.Publish(APIEvent.TaskTraced, APIEvent.NewEvent(APIEvent.TaskTraced, msg))
	b.log.Error("task un-assignable, failed plan", "task", taskID)
}

// onTaskTerminated drops the hiring state once a task reaches a terminal
// traced state (Done or Failed) on my plan.
func (b *board) onTaskTerminated(data any) {
	msg, ok := data.(APITaskmsg.TaskTracedMsg)
	if !ok || msg.Owner != string(b.tp.ID()) {
		return
	}
	if msg.State != APITask.StateDone && msg.State != APITask.StateFailed && msg.State != APITask.StateReleased {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// Released: the executor failed; reassign to the next candidate, or when
	// none remain emit the final Failed so the commander fails the plan.
	if msg.State == APITask.StateReleased {
		taskID := msg.TaskID
		hs, ok := b.myAds[taskID]
		if !ok {
			return
		}
		hiringCandidate := hs.candidate + 1
		if hiringCandidate >= firstCandidate || hiringCandidate >= len(hs.bestScores) {
			// Give up — drop the task and emit the terminal Failed. Deleted
			// first so the board ignores its own Failed emit.
			delete(b.myAds, taskID)
			fail := APITaskmsg.TaskTracedMsg{
				TaskID: taskID,
				PlanID: hs.task.TaskPlan.ID,
				State:  APITask.StateFailed,
				Owner:  hs.task.Meta.Owner,
			}
			_ = b.eb.Publish(APIEvent.TaskTraced, APIEvent.NewEvent(APIEvent.TaskTraced, fail))
			b.log.Error("no more candidates, failing plan", "task", taskID)
			return
		}
		hs.candidate++
		assign := APITaskmsg.TaskAssignMsg{
			TaskID: taskID,
			Spec:   hs.task.Spec,
			Prompt: hs.task.Spec.Prompt,
			Owner:  hs.task.Meta.Owner,
			PlanID: hs.task.TaskPlan.ID,
		}

		best := hs.bestScores[hiringCandidate].Best
		if best.PeerID == string(b.tp.ID()) {
			_ = b.eb.Publish(APIEvent.TaskAssigned, APIEvent.NewEvent(APIEvent.TaskAssigned, assign))
			b.log.Info("self-assigned (retry)", "task", taskID)
			return
		}
		if err := b.pm.SendEvent(b.ctx, APINode.NodeID(best.PeerID), APIEvent.TaskAssign, assign); err != nil {
			b.log.Error("assign failed", "task", taskID, "peer", best.PeerID, "err", err)
			return
		}
		b.log.Info("assigned to peer (retry)", "task", taskID, "peer", best.PeerID)
		return
	}

	// Done, or Failed (the board's own give-up) — clean up.
	delete(b.myAds, msg.TaskID)
}

// ── Employee side ──────────────────────────────────────────────────────

// onAd evaluates a peer's advertisement against local capability and bids.
func (b *board) onAd(msg *APITaskmsg.TaskBroadcastMsg) {
	if msg.Owner == string(b.tp.ID()) {
		return // my own broadcast
	}
	cv := b.buildCV(msg.TaskID)
	if matchCV(&cv, &msg.Spec) < 0 {
		b.log.Debug("ad rejected", "task", msg.TaskID, "spec", msg.Spec.Model)
		return
	}
	cv.PeerID = string(b.tp.ID())
	if err := b.pm.SendEvent(b.ctx, APINode.NodeID(msg.Owner), APIEvent.TaskCVReceived, APITaskmsg.TaskCVMsg{
		TaskID: msg.TaskID,
		CV:     cv,
	}); err != nil {
		b.log.Warn("bid send failed", "task", msg.TaskID, "err", err)
	}
	b.log.Info("bid sent", "task", msg.TaskID, "to", msg.Owner)
}

// onAssign re-publishes a remote assignment locally so the executor runs it.
func (b *board) onAssign(msg *APITaskmsg.TaskAssignMsg) {
	if msg.Owner == string(b.tp.ID()) {
		return // local assignment already dispatched
	}
	_ = b.eb.Publish(APIEvent.TaskAssigned, APIEvent.NewEvent(APIEvent.TaskAssigned, *msg))
	b.log.Info("received remote assignment", "task", msg.TaskID, "from", msg.Owner)
}

// buildCV assembles this node's capability profile.
func (b *board) buildCV(taskID APITask.TaskID) APICapability.CapabilityCV {
	cv := APICapability.CapabilityCV{
		TaskID: taskID,
		PeerID: string(b.tp.ID()),
	}
	for _, p := range b.pc.List() {
		for _, m := range p.GetModels() {
			cv.Models = append(cv.Models, m.ID)
		}
	}
	if b.hm != nil {
		cv.Hardware = b.hm.Snapshot()
	}
	return cv
}

// bestBid returns the highest-scoring qualifying bid, or nil if none.
// OPTIMIZE: bestBid
func bestBids(bids []APICapability.CapabilityCV, spec *APITask.TaskSpec) []APICapability.BestScore {
	var BestScores []APICapability.BestScore
	for i := range bids {
		score := matchCV(&bids[i], spec)
		if score < 0 {
			continue
		}

		BestScores = append(BestScores, APICapability.BestScore{
			Best:  &bids[i],
			Score: score,
		})
	}

	sort.Slice(BestScores, func(a, b int) bool {
		return BestScores[a].Score > BestScores[b].Score
	})

	if len(BestScores) > firstCandidate {
		BestScores = BestScores[0:firstCandidate]
	}
	return BestScores
}

// OPTIMIZE: score the CVs, needs to manage after the system is completed
// Match returns a score when the peer qualifies for the spec, or -1 when it
// is disqualified. Higher scores win the assignment.
//
// Checks, each skipped when the corresponding requirement is unset:
//   - VRAM: total free VRAM across GPUs must cover spec.MinVRAM.
//   - Model: the peer must serve spec.Model.
//   - Tools: every spec.Tools entry must be available.
//   - Labels: cv.Labels and spec.Labels must overlap (when both non-empty).
//
// The score rewards free memory and penalizes lower scheduling priority.
func matchCV(cv *APICapability.CapabilityCV, spec *APITask.TaskSpec) int {
	if spec.MinVRAM > 0 {
		var freeVRAM int64
		for _, g := range cv.Hardware.GPUSnapshot {
			freeVRAM += g.FreeVRAM
		}
		if freeVRAM < spec.MinVRAM {
			return -1
		}
	}

	if spec.Model != "" && len(cv.Models) > 0 && !APICapability.Contains(cv.Models, spec.Model) {
		return -1
	}

	if len(cv.Tools) > 0 {
		for _, t := range spec.Tools {
			if !APICapability.Contains(cv.Tools, t) {
				return -1
			}
		}
	}

	if len(cv.Labels) > 0 && len(spec.Labels) > 0 && !APICapability.AnyOverlap(cv.Labels, spec.Labels) {
		return -1
	}

	score := int(cv.Hardware.MemorySnapshot.FreeBytes / (1024 * 1024 * 1024))
	score -= spec.Priority
	return score
}

// ── AvailableModule Interface ──────────────────────────────────────────

func (b *board) GetModuleType() APIModule.ModuleType { return APIModule.TaskBoard }

func (b *board) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(b.GetModuleType(), "default")
}

func (b *board) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(b.GetModuleID(), b.GetModuleType(), APIModule.Running)
}

func (b *board) RegisterWithManager(m modulemanager.ModuleManager) error {
	b.mm = m
	return m.Register(b)
}

func (b *board) DependsOn() map[APIModule.ModuleType]string {
	return map[APIModule.ModuleType]string{
		APIModule.EventBus:           "default",
		APIModule.Transport:          "default",
		APIModule.TaskPostman:        "default",
		APIModule.ProviderController: "default",
	}
}

func (b *board) DependsEnable() error {
	for depType, name := range b.DependsOn() {
		id := APIModule.NewModuleID(depType, name)
		mod, err := b.mm.Get(id)
		if err != nil {
			return fmt.Errorf("resolve dependency %s: %w", id, err)
		}
		switch depType {
		case APIModule.EventBus:
			eb, ok := mod.(eventbus.EventBus)
			if !ok {
				return fmt.Errorf("dependency %s is not an EventBus", id)
			}
			b.eb = eb
		case APIModule.Transport:
			tp, ok := mod.(transport.Transport)
			if !ok {
				return fmt.Errorf("dependency %s is not a Transport", id)
			}
			b.tp = tp
		case APIModule.TaskPostman:
			pm, ok := mod.(taskpostman.TaskPostman)
			if !ok {
				return fmt.Errorf("dependency %s is not a TaskPostman", id)
			}
			b.pm = pm
		case APIModule.ProviderController:
			pc, ok := mod.(providerctrl.ProviderController)
			if !ok {
				return fmt.Errorf("dependency %s is not a ProviderController", id)
			}
			b.pc = pc
		}
	}
	// HardwareMonitor is optional (used only to enrich CV hardware).
	if mods := b.mm.GetByType(APIModule.HardwareMonitor); len(mods) > 0 {
		if hm, ok := mods[0].(hardwaremonitor.HardwareMonitor); ok {
			b.hm = hm
		}
	}
	return nil
}
