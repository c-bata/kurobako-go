package kurobako

import (
	"encoding/json"
	"fmt"
	"io"
)

type SolverSpec struct {
	Name         string            `json:"name"`
	Attrs        map[string]string `json:"attrs"`
	Capabilities Capabilities      `json:"capabilities"`
}

func NewSolverSpec(name string) SolverSpec {
	return SolverSpec{name, map[string]string{}, AllCapabilities}
}

type Solver interface {
	Ask(idg *TrialIdGenerator) (NextTrial, error)
	Tell(trial EvaluatedTrial) error
}

type SolverFactory interface {
	Specification() (*SolverSpec, error)
	CreateSolver(seed int64, problem ProblemSpec) (Solver, error)
}

type SolverRunner struct {
	factory SolverFactory
	solvers map[uint64]Solver
}

func NewSolverRunner(factory SolverFactory) *SolverRunner {
	return &SolverRunner{factory, nil}
}

func (r *SolverRunner) Run() error {
	r.solvers = map[uint64]Solver{}

	if err := r.castSolverSpec(); err != nil {
		return err
	}

	for {
		do_continue, err := r.runOnce()
		if err != nil {
			return err
		}

		if !do_continue {
			break
		}
	}

	return nil
}

func (r *SolverRunner) runOnce() (bool, error) {
	line, err := readLine()
	if err == io.EOF {
		return false, nil
	} else if err != nil {
		return false, err
	}

	var message map[string]interface{}
	if err := json.Unmarshal(line, &message); err != nil {
		return false, err
	}

	switch message["type"] {
	case "CREATE_SOLVER_CAST":
		err := r.handleCreateSolverCast(line)
		return true, err
	case "DROP_SOLVER_CAST":
		err := r.handleDropSolverCast(line)
		return true, err
	case "ASK_CALL":
		err := r.handleAskCall(line)
		return true, err
	case "TELL_CALL":
		err := r.handleTellCall(line)
		return true, err
	default:
		return false, fmt.Errorf("unknown message type: %v", message["type"])
	}
}

func (r *SolverRunner) handleTellCall(input []byte) error {
	var message struct {
		SolverId uint64         `json:"solver_id"`
		Trial    EvaluatedTrial `json:"trial"`
	}

	if err := json.Unmarshal(input, &message); err != nil {
		return err
	}

	solver := r.solvers[message.SolverId]
	if err := solver.Tell(message.Trial); err != nil {
		return err
	}

	reply := map[string]interface{}{
		"type": "TELL_REPLY",
	}
	return r.sendMessage(reply)
}

func (r *SolverRunner) handleAskCall(input []byte) error {
	var message struct {
		SolverId    uint64 `json:"solver_id"`
		NextTrialId uint64 `json:"next_trial_id"`
	}

	if err := json.Unmarshal(input, &message); err != nil {
		return err
	}

	idg := TrialIdGenerator{message.NextTrialId}
	solver := r.solvers[message.SolverId]
	trial, err := solver.Ask(&idg)
	if err != nil {
		return err
	}

	reply := map[string]interface{}{
		"type":          "ASK_REPLY",
		"trial":         trial,
		"next_trial_id": idg.NextId,
	}
	return r.sendMessage(reply)
}

func (r *SolverRunner) handleDropSolverCast(input []byte) error {
	var message struct {
		SolverId uint64 `json:"solver_id"`
	}

	if err := json.Unmarshal(input, &message); err != nil {
		return err
	}

	delete(r.solvers, message.SolverId)
	return nil
}

func (r *SolverRunner) handleCreateSolverCast(input []byte) error {
	var message struct {
		SolverId   uint64      `json:"solver_id"`
		RandomSeed uint64      `json:"random_seed"`
		Problem    ProblemSpec `json:"problem"`
	}

	if err := json.Unmarshal(input, &message); err != nil {
		return err
	}

	solver, err := r.factory.CreateSolver(int64(message.RandomSeed), message.Problem)
	if err != nil {
		return err
	}

	r.solvers[message.SolverId] = solver
	return nil
}

func (r *SolverRunner) castSolverSpec() error {
	spec, err := r.factory.Specification()
	if err != nil {
		return err
	}

	return r.sendMessage(map[string]interface{}{"type": "SOLVER_SPEC_CAST", "spec": spec})
}

func (r *SolverRunner) sendMessage(message map[string]interface{}) error {
	bytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", string(bytes))
	return nil
}