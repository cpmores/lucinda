// Package taskboard manages distributed task advertisements and bidding.
// It listens for local ready tasks and broadcasts them, and handles incoming
// ads from peers by submitting CapabilityCV bids.
package taskboard

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	APICapability "github.com/cpmores/lucinda/api/v1/capability"
	APIEvent "github.com/cpmores/lucinda/api/v1/event"
	APIModule "github.com/cpmores/lucinda/api/v1/module"
	apinode "github.com/cpmores/lucinda/api/v1/node"
	APITask "github.com/cpmores/lucinda/api/v1/task"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/taskmsg"
	taskpostman "github.com/cpmores/lucinda/internel/task_management_layer/task_postman"
	taskstatemanager "github.com/cpmores/lucinda/internel/task_management_layer/task_state_manager"
	tasktracer "github.com/cpmores/lucinda/internel/task_management_layer/task_tracer"
	hardwaremonitor "github.com/cpmores/lucinda/pkg/infrastructure_layer/hardware_monitor"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	providercontroller "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider"
	transport "github.com/cpmores/lucinda/pkg/infrastructure_layer/transport"
)

const (
	defaultInterviewTime int64 = 5 // seconds to collect bids
	TaskBoardProtocol          = "/lucinda/taskboard/1.0.0"
)

type TaskBoard interface {
	Start(ctx context.Context) error
	Stop() error

	// ── Incoming (from other nodes) ─────────────────────────────────────
	Drawup(ad *APITask.TaskAd) error   // load to ads
	Ripup(taskID APITask.TaskID) error // remove an ad
	Putup(taskID APITask.TaskID) error // advertise my own ready node

	// ── Bidding ─────────────────────────────────────────────────────────
	Handout(taskID APITask.TaskID, cv *APICapability.CapabilityCV) error  // peer bid on my ad
	Interview(taskID APITask.TaskID) (*APICapability.CapabilityCV, error) // pick best bid
}

type board struct {
	mu sync.RWMutex
	mm modulemanager.ModuleManager
	tp transport.Transport
	pm taskpostman.Postman
	tt tasktracer.TaskTracer
	pc providercontroller.ProviderController
	sm taskstatemanager.TaskStateManager
	hm hardwaremonitor.HardwareMonitor

	// My ads → broadcast to peers (inbound from StateManager).
	myAds map[APITask.TaskID]*APITask.TaskAd

	// Ads from other peers → I might bid on these.
	peerAds map[APITask.TaskID]*APITask.TaskAd

	// Bids: taskID → list of CVs submitted by peers (on my ads).
	bids map[APITask.TaskID][]*APICapability.CapabilityCV

	cancel context.CancelFunc
}

func NewTaskBoard(mm modulemanager.ModuleManager) TaskBoard {
	postmans := mm.GetByType(APIModule.TASKPOSTMAN)
	if len(postmans) == 0 {
		log.Fatal("taskboard: no TaskPostman module found")
	}
	postman := postmans[0].(taskpostman.Postman)

	transports := mm.GetByType(APIModule.TRANSPORT)
	if len(transports) == 0 {
		log.Fatal("taskboard: no Transport module found")
	}
	transport := transports[0].(transport.Transport)

	taskTracers := mm.GetByType(APIModule.TASKTRACER)
	if len(taskTracers) == 0 {
		log.Fatal("taskboard: no TaskTracer module found")
	}
	taskTracer := taskTracers[0].(tasktracer.TaskTracer)

	providerControllers := mm.GetByType(APIModule.PROVIDERCONTROLLER)
	if len(providerControllers) == 0 {
		log.Fatal("taskboard: no ProviderController module found")
	}
	providerController := providerControllers[0].(providercontroller.ProviderController)

	hardwareMonitors := mm.GetByType(APIModule.HARDWAREMONITOR)
	if len(hardwareMonitors) == 0 {
		log.Fatal("taskboard: no HardwareMonitor module found")
	}
	hardwareMonitor := hardwareMonitors[0].(hardwaremonitor.HardwareMonitor)

	stateManagers := mm.GetByType(APIModule.TASKSTATEMANAGER)
	if len(stateManagers) == 0 {
		log.Fatal("taskboard: no TaskStateManager module found")
	}
	stateManager := stateManagers[0].(taskstatemanager.TaskStateManager)

	return &board{
		mm: mm,
		tp: transport,
		pm: postman,
		tt: taskTracer,
		pc: providerController,
		hm: hardwareMonitor,
		sm: stateManager,

		myAds:   make(map[APITask.TaskID]*APITask.TaskAd),
		peerAds: make(map[APITask.TaskID]*APITask.TaskAd),
		bids:    make(map[APITask.TaskID][]*APICapability.CapabilityCV),
	}
}

// ── Lifecycle ─────────────────────────────────────────────────────────

func (b *board) Start(ctx context.Context) error {
	ctx, b.cancel = context.WithCancel(ctx)

	//
	b.pm.Deliver(ctx, b.tp, TaskBoardProtocol)
	// When local StateManager says a node is Ready, broadcast it.
	b.pm.Watch(ctx, APIEvent.TaskReady, func(data any) error {
		node, ok := data.(*APITask.TaskNode)
		if !ok {
			return nil
		}
		return b.Putup(node.ID)
	})

	// Rebroadcast previously expired/failed nodes.
	b.pm.Watch(ctx, APIEvent.TaskRepublished, func(data any) error {
		node, ok := data.(*APITask.TaskNode)
		if !ok {
			return nil
		}
		return b.Putup(node.ID)
	})

	// Receive ads from other nodes
	b.pm.Watch(ctx, APIEvent.TaskAdReceived, func(data any) error {
		node, ok := data.(*APITask.TaskAd)
		if !ok {
			return nil
		}

		return b.Drawup(node)
	})

	// Receive CV bids from peers on my ads.
	b.pm.Watch(ctx, APIEvent.TaskCVReceived, func(data any) error {
		node, ok := data.(*APITaskmsg.TaskRequestMsg)
		if !ok {
			return nil
		}

		return b.Handout(APITask.TaskID(node.CV.TaskID), &node.CV)
	})

	go b.heartbeat(ctx)
	log.Println("taskboard: started")
	return nil
}

func (b *board) Stop() error {
	b.pm.Stop()
	if b.cancel != nil {
		b.cancel()
	}
	log.Println("taskboard: stopped")
	return nil
}

// ── Drawup — incoming ad from a peer ──────────────────────────────────

func (b *board) Drawup(ad *APITask.TaskAd) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.peerAds[ad.ID]; ok {
		return nil
	}
	b.peerAds[ad.ID] = ad
	b.submitBid(*ad)
	return nil
}

// ── Ripup — remove an ad ──────────────────────────────────────────────

func (b *board) Ripup(taskID APITask.TaskID) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.peerAds, taskID)
	delete(b.bids, taskID)
	return nil
}

// ── Putup — advertise my node to peers ─────────────────────────────────

func (b *board) Putup(taskID APITask.TaskID) error {
	b.mu.Lock()
	if _, ok := b.myAds[taskID]; ok {
		b.mu.Unlock()
		return nil
	}

	task, err := b.tt.GetLocal(taskID)
	if err != nil {
		b.mu.Unlock()
		return fmt.Errorf("putup: %w", err)
	}

	ad := APITask.TaskToTaskAd(task)
	b.myAds[taskID] = &ad
	b.mu.Unlock()

	// Broadcast to peers via Transport. Their Postman.Deliver picks it up,
	// publishes to TaskAdReceived, and their Drawup handles it.
	return b.tp.Publish(context.Background(), apinode.NewNodeMessage(
		TaskBoardProtocol,
		string(APIEvent.TaskAdReceived),
		apinode.NodeID(""), // from is filled by transport
		apinode.NodeID(""), // to all
		APITaskmsg.TaskAdToTaskBroadcastMsg(&ad),
	))
}

// ── Handout — peer submitted a CV bid on my ad ─────────────────────────

func (b *board) Handout(taskID APITask.TaskID, cv *APICapability.CapabilityCV) error {
	b.mu.Lock()
	b.bids[taskID] = append(b.bids[taskID], cv)
	first := len(b.bids[taskID]) == 1
	b.mu.Unlock()

	if first {
		go func() {
			time.Sleep(time.Duration(defaultInterviewTime) * time.Second)
			b.Interview(taskID)
		}()
	}
	return nil
}

// ── Interview — pick the best bid ──────────────────────────────────────

func (b *board) Interview(taskID APITask.TaskID) (*APICapability.CapabilityCV, error) {
	task, err := b.tt.GetLocal(taskID)
	if err != nil {
		return nil, fmt.Errorf("interview: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	candidates := b.bids[taskID]
	if len(candidates) == 0 {
		return nil, nil
	}

	ad, ok := b.myAds[taskID]
	if !ok {
		return nil, nil
	}

	type entry struct {
		cv    *APICapability.CapabilityCV
		score int
	}
	var ranked []entry
	for _, cv := range candidates {
		score := cv.Match(&ad.Spec)
		if score >= 0 {
			ranked = append(ranked, entry{cv, score})
		}
	}

	if len(ranked) == 0 {
		log.Printf("taskboard: no qualified bids for %s", taskID)
		return nil, nil
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	winner := ranked[0].cv
	log.Printf("taskboard: task %s awarded to %s (score %d)", taskID, winner.PeerID, ranked[0].score)

	assign := APITaskmsg.TaskToTaskAssignMsg(task)
	b.tp.Send(context.Background(), apinode.NodeID(winner.PeerID), apinode.NewNodeMessage(
		TaskBoardProtocol,
		string(APIEvent.TaskAssigned),
		apinode.NodeID(""),
		apinode.NodeID(winner.PeerID),
		assign,
	))

	delete(b.bids, taskID)
	delete(b.myAds, taskID)
	return winner, nil
}

// ── Queries ────────────────────────────────────────────────────────────

// ── Heartbeat — expire stale lease claims ──────────────────────────

func (b *board) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, node := range b.sm.Expired() {
				log.Printf("taskboard: node %s expired, rebroadcasting", node.ID)
				b.Putup(node.ID)
			}
		}
	}
}

func (b *board) Wanted() ([]*APITask.Task, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]*APITask.Task, 0, len(b.peerAds))
	for _, ad := range b.peerAds {
		result = append(result, &APITask.Task{
			Meta: APITask.TaskMeta{ID: ad.ID},
			Spec: ad.Spec,
		})
	}
	return result, nil
}

func (b *board) Posted() ([]*APITask.Task, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]*APITask.Task, 0, len(b.myAds))
	for _, ad := range b.myAds {
		result = append(result, &APITask.Task{
			Meta: APITask.TaskMeta{ID: ad.ID},
			Spec: ad.Spec,
		})
	}
	return result, nil
}

// ── Internal ───────────────────────────────────────────────────────────

func (b *board) submitBid(ad APITask.TaskAd) {
	cv := b.buildCV(ad)
	if cv.Match(&ad.Spec) < 0 {
		return
	}
	// In production: Transport.Send(origin, TaskRequestMsg{ad.ID, cv})
	log.Printf("taskboard: submitted bid for %s", ad.ID)
	b.tp.Send(context.Background(), apinode.NodeID(ad.Owner), apinode.NewNodeMessage(
		TaskBoardProtocol,
		string(APIEvent.TaskCVReceived),
		apinode.NodeID(""),
		apinode.NodeID(ad.Owner),
		APITaskmsg.TaskCVToTaskRequestMsg(cv),
	))
}

func (b *board) buildCV(ad APITask.TaskAd) *APICapability.CapabilityCV {
	hardware := b.hm.Snapshot()
	hardware.GPUSnapshot = append(hardware.GPUSnapshot, b.pc.GPU())
	// TODO: add tools labels models
	return &APICapability.CapabilityCV{
		TaskID:   ad.ID,
		PeerID:   string(b.tp.ID()),
		Hardware: hardware,
	}
}
