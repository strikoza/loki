package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/grafana/loki/v3/pkg/blockbuilder/types"
)

type mockOffsetReader struct {
	groupLag map[int32]kadm.GroupMemberLag
}

func (m *mockOffsetReader) GroupLag(_ context.Context) (map[int32]kadm.GroupMemberLag, error) {
	return m.groupLag, nil
}

func TestRecordCountPlanner_Plan(t *testing.T) {
	for _, tc := range []struct {
		name         string
		recordCount  int64
		expectedJobs []*JobWithPriority[int]
		groupLag     map[int32]kadm.GroupMemberLag
	}{
		{
			name:        "single partition, single job",
			recordCount: 100,
			groupLag: map[int32]kadm.GroupMemberLag{
				0: {
					Commit: kadm.Offset{
						At: 100,
					},
					End: kadm.ListedOffset{
						Offset: 150,
					},
					Partition: 0,
				},
			},
			expectedJobs: []*JobWithPriority[int]{
				NewJobWithPriority(
					types.NewJob(0, types.Offsets{Min: 101, Max: 150}),
					49, // 150-101
				),
			},
		},
		{
			name:        "single partition, multiple jobs",
			recordCount: 50,
			groupLag: map[int32]kadm.GroupMemberLag{
				0: {
					Commit: kadm.Offset{
						At: 100,
					},
					End: kadm.ListedOffset{
						Offset: 200,
					},
					Partition: 0,
				},
			},
			expectedJobs: []*JobWithPriority[int]{
				NewJobWithPriority(
					types.NewJob(0, types.Offsets{Min: 101, Max: 151}),
					99, // priority is total remaining: 200-101
				),
				NewJobWithPriority(
					types.NewJob(0, types.Offsets{Min: 151, Max: 200}),
					49, // priority is total remaining: 200-151
				),
			},
		},
		{
			name:        "multiple partitions",
			recordCount: 100,
			groupLag: map[int32]kadm.GroupMemberLag{
				0: {
					Commit: kadm.Offset{
						At: 100,
					},
					End: kadm.ListedOffset{
						Offset: 150,
					},
					Partition: 0,
				},
				1: {
					Commit: kadm.Offset{
						At: 200,
					},
					End: kadm.ListedOffset{
						Offset: 400,
					},
					Partition: 1,
				},
			},
			expectedJobs: []*JobWithPriority[int]{
				NewJobWithPriority(
					types.NewJob(1, types.Offsets{Min: 201, Max: 301}),
					199, // priority is total remaining: 400-201
				),
				NewJobWithPriority(
					types.NewJob(1, types.Offsets{Min: 301, Max: 400}),
					99, // priority is total remaining: 400-301
				),
				NewJobWithPriority(
					types.NewJob(0, types.Offsets{Min: 101, Max: 150}),
					49, // priority is total remaining: 150-101
				),
			},
		},
		{
			name:        "no lag",
			recordCount: 100,
			groupLag: map[int32]kadm.GroupMemberLag{
				0: {
					Commit: kadm.Offset{
						At: 100,
					},
					End: kadm.ListedOffset{
						Offset: 100,
					},
					Partition: 0,
				},
			},
			expectedJobs: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockReader := &mockOffsetReader{
				groupLag: tc.groupLag,
			}
			cfg := Config{
				Interval:          time.Second, // foced > 0 in validation
				Strategy:          RecordCountStrategy,
				TargetRecordCount: tc.recordCount,
			}
			require.NoError(t, cfg.Validate())

			planner := NewRecordCountPlanner(tc.recordCount)
			planner.offsetReader = mockReader

			jobs, err := planner.Plan(context.Background())
			require.NoError(t, err)

			require.Equal(t, len(tc.expectedJobs), len(jobs))
			require.ElementsMatch(t, tc.expectedJobs, jobs)
		})
	}
}