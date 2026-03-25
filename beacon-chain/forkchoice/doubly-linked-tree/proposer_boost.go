package doublylinkedtree

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/prysmaticlabs/prysm/v5/attacker"
	"github.com/sirupsen/logrus"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
)

// applyProposerBoostScore applies the current proposer boost scores to the
// relevant nodes.
func (f *ForkChoice) applyProposerBoostScore() error {
	s := f.store
	proposerScore := uint64(0)
	if s.previousProposerBoostRoot != params.BeaconConfig().ZeroHash {
		previousNode, ok := s.nodeByRoot[s.previousProposerBoostRoot]
		if !ok || previousNode == nil {
			log.WithError(errInvalidProposerBoostRoot).Errorf(fmt.Sprintf("invalid prev root %#x", s.previousProposerBoostRoot))
		} else {
			previousNode.balance -= s.previousProposerBoostScore
		}
	}

	if s.proposerBoostRoot != params.BeaconConfig().ZeroHash {
		currentNode, ok := s.nodeByRoot[s.proposerBoostRoot]
		if !ok || currentNode == nil {
			log.WithError(errInvalidProposerBoostRoot).Errorf(fmt.Sprintf("invalid current root %#x", s.proposerBoostRoot))
		} else {
			proposerScore = (s.committeeWeight * params.BeaconConfig().ProposerScoreBoost) / 100
			currentNode.balance += proposerScore
		}
	}

	attclient := attacker.GetAttacker()
	type WeightAndSlotRoot struct {
		SlotRoot string `json:"slot_root"`
		Weight   int64  `json:"weight"`
	}
	if attclient != nil {
		res, err := attclient.ModifyBlockWeight(context.Background())
		if err != nil {
			log.WithError(err).Error("failed to get special weight and slot root")
		} else if res.Result != "" {
			var weightAndRoot WeightAndSlotRoot
			json.Unmarshal([]byte(res.Result), &weightAndRoot)
			slotRoot, _ := attacker.FromHex(weightAndRoot.SlotRoot)
			var specialRoot [fieldparams.RootLength]byte
			copy(specialRoot[:], slotRoot)
			{
				specialNode, ok := s.nodeByRoot[specialRoot]
				if !ok || specialNode == nil {
					log.WithFields(logrus.Fields{
						"response_root": weightAndRoot.SlotRoot,
						"special_root":  hex.EncodeToString(specialRoot[:]),
						"weight":        weightAndRoot.Weight,
						"err":           errInvalidProposerBoostRoot,
					}).Error("update special root weight failed")
				} else {

					if weightAndRoot.Weight < 0 {
						specialNode.balance -= s.committeeWeight * uint64(-weightAndRoot.Weight)
					} else {
						specialNode.balance += s.committeeWeight * uint64(weightAndRoot.Weight)
					}
					log.WithFields(logrus.Fields{
						"response_root": weightAndRoot.SlotRoot,
						"special_root":  hex.EncodeToString(specialRoot[:]),
						"weight":        weightAndRoot.Weight,
					}).Info("update special root weight succeed")
				}
			}
		}
	}

	s.previousProposerBoostRoot = s.proposerBoostRoot
	s.previousProposerBoostScore = proposerScore
	return nil
}

// ProposerBoost of fork choice store.
func (s *Store) proposerBoost() [fieldparams.RootLength]byte {
	return s.proposerBoostRoot
}
