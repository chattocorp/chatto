package graph

import (
	"strconv"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

func natsStatsToModel(stats *core.JetStreamStats) *model.NatsStats {
	if stats == nil {
		return &model.NatsStats{
			Streams:   []*model.NatsStreamInfo{},
			Consumers: []*model.NatsConsumerInfo{},
		}
	}

	streams := make([]*model.NatsStreamInfo, 0, len(stats.Streams))
	for _, stream := range stats.Streams {
		streams = append(streams, &model.NatsStreamInfo{
			Name:          stream.Name,
			Description:   stream.Description,
			Subjects:      append([]string(nil), stream.Subjects...),
			Storage:       stream.Storage,
			Messages:      int(stream.Messages),
			Bytes:         int(stream.Bytes),
			FirstSequence: strconv.FormatUint(stream.FirstSeq, 10),
			LastSequence:  strconv.FormatUint(stream.LastSeq, 10),
			ConsumerCount: int32(stream.ConsumerCount),
			Replicas:      int32(stream.Replicas),
			ClusterLeader: stream.ClusterLeader,
		})
	}

	consumers := make([]*model.NatsConsumerInfo, 0, len(stats.Consumers))
	for _, consumer := range stats.Consumers {
		consumers = append(consumers, &model.NatsConsumerInfo{
			Stream:                    consumer.Stream,
			Name:                      consumer.Name,
			Durable:                   consumer.Durable,
			FilterSubject:             consumer.FilterSubject,
			FilterSubjects:            append([]string(nil), consumer.FilterSubjects...),
			AckPolicy:                 consumer.AckPolicy,
			PullBased:                 consumer.PullBased,
			PushBound:                 consumer.PushBound,
			Pending:                   int(consumer.Pending),
			AckPending:                int32(consumer.AckPending),
			Redelivered:               int32(consumer.Redelivered),
			Waiting:                   int32(consumer.Waiting),
			DeliveredConsumerSequence: strconv.FormatUint(consumer.DeliveredConsumerSeq, 10),
			DeliveredStreamSequence:   strconv.FormatUint(consumer.DeliveredStreamSeq, 10),
			AckFloorConsumerSequence:  strconv.FormatUint(consumer.AckFloorConsumerSeq, 10),
			AckFloorStreamSequence:    strconv.FormatUint(consumer.AckFloorStreamSeq, 10),
		})
	}

	return &model.NatsStats{
		Streams:              streams,
		Consumers:            consumers,
		TotalMessages:        int(stats.TotalMessages),
		TotalBytes:           int(stats.TotalBytes),
		TotalConsumerPending: int(stats.TotalConsumerPending),
		TotalAckPending:      int32(stats.TotalAckPending),
	}
}
