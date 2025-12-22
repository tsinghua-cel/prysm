package validator

import (
	"bufio"
	"context"
	"fmt"
	"github.com/OffchainLabs/prysm/v7/attacker"
	attackclient "github.com/tsinghua-cel/attacker-client-go/client"
	"os"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// GetAttestationData requests that the beacon node produce an attestation data object,
// which the validator acting as an attester will then sign.
func (vs *Server) GetAttestationData(ctx context.Context, req *ethpb.AttestationDataRequest) (*ethpb.AttestationData, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.RequestAttestation")
	defer span.End()
	span.SetAttributes(
		trace.Int64Attribute("slot", int64(req.Slot)),
		trace.Int64Attribute("committeeIndex", int64(req.CommitteeIndex)),
	)

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	// luxq: add attestation verify time cost.
	t1 := time.Now()
	defer func() {
		t2 := time.Now()
		file, err := os.OpenFile("/root/beacondata/GetAttest.csv", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			log.WithError(err).Error("Failed to create file for GetAttest.csv")
		} else {
			write := bufio.NewWriter(file)
			write.WriteString(fmt.Sprintf("%d,%d\n", int64(req.Slot), t2.Sub(t1).Microseconds()))
			write.Flush()
			file.Close()
		}
	}()

	res, err := vs.CoreService.GetAttestationData(ctx, req)
	if err != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(err.Reason), "Could not get attestation data: %v", err.Err)
	}
	return res, nil
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// ProposeAttestation is a function called by an attester to vote
// on a block via an attestation object as defined in the Ethereum specification.
func (vs *Server) ProposeAttestation(ctx context.Context, att *ethpb.Attestation) (*ethpb.AttestResponse, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.ProposeAttestation")
	defer span.End()

	resp, err := vs.proposeAtt(ctx, att, att.GetData().CommitteeIndex)
	if err != nil {
		return nil, err
	}

	if features.Get().EnableExperimentalAttestationPool {
		if err = vs.AttestationCache.Add(att); err != nil {
			log.WithError(err).Error("Could not save attestation")
		}
	} else {
		go func() {
			attCopy := att.Copy()
			if err := vs.AttPool.SaveUnaggregatedAttestation(attCopy); err != nil {
				log.WithError(err).Error("Could not save unaggregated attestation")
				return
			}
		}()
	}

	return resp, nil
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// ProposeAttestationElectra is a function called by an attester to vote
// on a block via an attestation object as defined in the Ethereum specification.
func (vs *Server) ProposeAttestationElectra(ctx context.Context, singleAtt *ethpb.SingleAttestation) (*ethpb.AttestResponse, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.ProposeAttestationElectra")
	defer span.End()

	resp, err := vs.proposeAtt(ctx, singleAtt, singleAtt.GetCommitteeIndex())
	if err != nil {
		return nil, err
	}

	targetState, err := vs.AttestationStateFetcher.AttestationTargetState(ctx, singleAtt.Data.Target)
	if err != nil {
		return nil, status.Error(codes.Internal, "Could not get target state")
	}
	committee, err := helpers.BeaconCommitteeFromState(ctx, targetState, singleAtt.Data.Slot, singleAtt.GetCommitteeIndex())
	if err != nil {
		return nil, status.Error(codes.Internal, "Could not get committee")
	}

	singleAttCopy := singleAtt.Copy()
	att := singleAttCopy.ToAttestationElectra(committee)
	if features.Get().EnableExperimentalAttestationPool {
		if err = vs.AttestationCache.Add(att); err != nil {
			log.WithError(err).Error("Could not save attestation")
		}
	} else {
		go func() {
			if err := vs.AttPool.SaveUnaggregatedAttestation(att); err != nil {
				log.WithError(err).Error("Could not save unaggregated attestation")
				return
			}
		}()
	}

	return resp, nil
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// SubscribeCommitteeSubnets subscribes to the committee ID subnet given subscribe request.
func (vs *Server) SubscribeCommitteeSubnets(ctx context.Context, req *ethpb.CommitteeSubnetsSubscribeRequest) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.SubscribeCommitteeSubnets")
	defer span.End()

	if len(req.Slots) != len(req.CommitteeIds) || len(req.CommitteeIds) != len(req.IsAggregator) {
		return nil, status.Error(codes.InvalidArgument, "request fields are not the same length")
	}
	if len(req.Slots) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no attester slots provided")
	}

	fetchValsLen := func(slot primitives.Slot) (uint64, error) {
		wantedEpoch := slots.ToEpoch(slot)
		vals, err := vs.HeadFetcher.HeadValidatorsIndices(ctx, wantedEpoch)
		if err != nil {
			return 0, err
		}
		return uint64(len(vals)), nil
	}

	// Request the head validator indices of epoch represented by the first requested
	// slot.
	currValsLen, err := fetchValsLen(req.Slots[0])
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not retrieve head validator length: %v", err)
	}
	currEpoch := slots.ToEpoch(req.Slots[0])

	for i := 0; i < len(req.Slots); i++ {
		// If epoch has changed, re-request active validators length
		if currEpoch != slots.ToEpoch(req.Slots[i]) {
			currValsLen, err = fetchValsLen(req.Slots[i])
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not retrieve head validator length: %v", err)
			}
			currEpoch = slots.ToEpoch(req.Slots[i])
		}
		subnet := helpers.ComputeSubnetFromCommitteeAndSlot(currValsLen, req.CommitteeIds[i], req.Slots[i])
		cache.SubnetIDs.AddAttesterSubnetID(req.Slots[i], subnet)
		if req.IsAggregator[i] {
			cache.SubnetIDs.AddAggregatorSubnetID(req.Slots[i], subnet)
		}
	}

	return &emptypb.Empty{}, nil
}

func (vs *Server) proposeAtt(
	ctx context.Context,
	att ethpb.Att,
	committeeIndex primitives.CommitteeIndex,
) (*ethpb.AttestResponse, error) {
	if _, err := bls.SignatureFromBytes(att.GetSignature()); err != nil {
		return nil, status.Error(codes.InvalidArgument, "Incorrect attestation signature")
	}

	root, err := att.GetData().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get attestation root: %v", err)
	}

	if att.Version() < version.Electra && slots.ToEpoch(vs.TimeFetcher.CurrentSlot()) >= params.BeaconConfig().ElectraForkEpoch {
		return nil, status.Error(codes.InvalidArgument, "old attestation format, ProposeAttestationElectra should be called post Electra")
	}

	if att.Version() >= version.Electra && slots.ToEpoch(vs.TimeFetcher.CurrentSlot()) < params.BeaconConfig().ElectraForkEpoch {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("ProposeAttestationElectra not supported yet. The current epoch is %d supported starting epoch is %d", slots.ToEpoch(vs.TimeFetcher.CurrentSlot()), params.BeaconConfig().ElectraForkEpoch))
	}

	// Broadcast the unaggregated attestation on a feed to notify other services in the beacon node
	// of a received unaggregated attestation.
	if att.IsSingle() {
		vs.OperationNotifier.OperationFeed().Send(&feed.Event{
			Type: operation.SingleAttReceived,
			Data: &operation.SingleAttReceivedData{
				Attestation: att,
			},
		})
	} else {
		vs.OperationNotifier.OperationFeed().Send(&feed.Event{
			Type: operation.UnaggregatedAttReceived,
			Data: &operation.UnAggregatedAttReceivedData{
				Attestation: att,
			},
		})
	}

	// Determine subnet to broadcast attestation to
	wantedEpoch := slots.ToEpoch(att.GetData().Slot)
	vals, err := vs.HeadFetcher.HeadValidatorsIndices(ctx, wantedEpoch)
	if err != nil {
		return nil, err
	}
	subnet := helpers.ComputeSubnetFromCommitteeAndSlot(uint64(len(vals)), committeeIndex, att.GetData().Slot)

	go func() {
		client := attacker.GetAttacker()
		skipBroadCast := false
		if client != nil {
			ctx = context.Background()
			var res attackclient.AttackerResponse
			res, err = client.AttestBeforeBroadCast(context.Background(), uint64(att.GetData().Slot))
			if err != nil {
				log.WithField("attacker", "delay").WithField("error", err).Error("An error occurred while AttestBeforeBroadCast")
			} else {
				log.WithField("attacker", "AttestBeforeBroadCast").Info("attacker succeed")
			}
			switch res.Cmd {
			case attackclient.CMD_EXIT, attackclient.CMD_ABORT:
				os.Exit(-1)
			case attackclient.CMD_SKIP:
				skipBroadCast = true
			case attackclient.CMD_RETURN:
				log.WithField("attacker", "AttestBeforeBroadCast").Warn("broadcast interrupt by attacker")
				// just return.
			case attackclient.CMD_NULL, attackclient.CMD_CONTINUE:
				// do nothing.
			}
		}

		if !skipBroadCast {
			// Broadcast the new attestation to the network.
			if err := vs.P2P.BroadcastAttestation(ctx, subnet, att); err != nil {
				log.WithError(err).Error("Could not broadcast attestation")
				//return nil, status.Errorf(codes.Internal, "Could not broadcast attestation: %v", err)
			}

		}
		if client != nil {
			var res attackclient.AttackerResponse
			res, err = client.AttestAfterBroadCast(context.Background(), uint64(att.GetData().Slot))
			if err != nil {
				log.WithField("attacker", "delay").WithField("error", err).Error("An error occurred while AttestAfterBroadCast")
			} else {
				log.WithField("attacker", "AttestAfterBroadCast").Info("attacker succeed")
			}
			switch res.Cmd {
			case attackclient.CMD_EXIT, attackclient.CMD_ABORT:
				os.Exit(-1)
			case attackclient.CMD_SKIP:
				// just nothing to do.
			case attackclient.CMD_RETURN:
				return
			case attackclient.CMD_NULL, attackclient.CMD_CONTINUE:
				// do nothing.
			}
		}
	}()

	return &ethpb.AttestResponse{
		AttestationDataRoot: root[:],
	}, nil
}
