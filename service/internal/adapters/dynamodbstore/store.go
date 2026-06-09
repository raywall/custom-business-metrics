package dynamodbstore

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/raywall/custom-business-metrics/service/internal/core"
)

// Store persists metrics in DynamoDB and keeps dashboard definitions in memory for the MVP.
type Store struct {
	client     *dynamodb.Client
	table      string
	mu         sync.RWMutex
	dashboards map[string]core.Dashboard
}

type storedMetric struct {
	PK            string `dynamodbav:"pk"`
	SK            string `dynamodbav:"sk"`
	ID            string `dynamodbav:"id"`
	MetricName    string `dynamodbav:"metric_name"`
	Timestamp     string `dynamodbav:"timestamp"`
	TimestampUnix int64  `dynamodbav:"timestamp_unix"`
	ExpiresAt     int64  `dynamodbav:"expires_at"`
	CorrelationID string `dynamodbav:"correlation_id,omitempty"`
	TraceID       string `dynamodbav:"trace_id,omitempty"`
	EventJSON     string `dynamodbav:"event_json"`
}

// NewStore creates a DynamoDB-backed metric store.
func NewStore(ctx context.Context, table, region, endpoint string) (*Store, error) {
	if region == "" {
		region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("local", "local", "")),
	)
	if err != nil {
		return nil, err
	}
	client := dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
		if endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	return &Store{client: client, table: table, dashboards: seedDashboards()}, nil
}

// SaveMetrics stores metric events with a DynamoDB TTL.
func (s *Store) SaveMetrics(ctx context.Context, events []core.MetricEvent, retentionDays int) error {
	expiresAt := time.Now().UTC().Add(time.Duration(retentionDays) * 24 * time.Hour)
	for _, event := range events {
		id := eventID()
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		item, err := attributevalue.MarshalMap(storedMetric{
			PK:            "metric#" + event.Name,
			SK:            event.Timestamp.Format(time.RFC3339Nano) + "#" + id,
			ID:            id,
			MetricName:    event.Name,
			Timestamp:     event.Timestamp.Format(time.RFC3339Nano),
			TimestampUnix: event.Timestamp.Unix(),
			ExpiresAt:     expiresAt.Unix(),
			CorrelationID: event.Tags["correlation_id"],
			TraceID:       event.TraceID,
			EventJSON:     string(body),
		})
		if err != nil {
			return err
		}
		if _, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(s.table), Item: item}); err != nil {
			return err
		}
	}
	return nil
}

// ListEvents returns raw stored metric events.
func (s *Store) ListEvents(ctx context.Context, filter core.MetricFilter, limit int) ([]core.MetricEventRecord, error) {
	events, err := s.scanEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

// ListSummaries returns aggregates grouped by metric name and optional groupBy.
func (s *Store) ListSummaries(ctx context.Context, filter core.MetricFilter) ([]core.MetricSummary, error) {
	events, err := s.scanEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*core.MetricSummary{}
	for _, record := range events {
		event := record.Event
		group := groupValue(event, filter.GroupBy)
		key := event.Name + "\x00" + filter.GroupBy + "\x00" + group
		summary := byKey[key]
		if summary == nil {
			summary = &core.MetricSummary{Name: event.Name, Group: group, GroupBy: filter.GroupBy, Kind: event.Kind, Unit: event.Unit}
			byKey[key] = summary
		}
		summary.Count++
		summary.Sum += event.Value
		if event.Timestamp.After(summary.UpdatedAt) {
			summary.UpdatedAt = event.Timestamp
			summary.Latest = event.Value
		}
	}
	out := make([]core.MetricSummary, 0, len(byKey))
	for _, summary := range byKey {
		if summary.Count > 0 {
			summary.Average = summary.Sum / float64(summary.Count)
		}
		out = append(out, *summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Group < out[j].Group
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// QuerySeries returns sum/count points grouped by bucket.
func (s *Store) QuerySeries(ctx context.Context, filter core.MetricFilter, bucket time.Duration) ([]core.MetricPoint, error) {
	events, err := s.scanEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	byBucket := map[int64]*core.MetricPoint{}
	for _, record := range events {
		event := record.Event
		key := event.Timestamp.Truncate(bucket).Unix()
		point := byBucket[key]
		if point == nil {
			point = &core.MetricPoint{Bucket: time.Unix(key, 0).UTC()}
			byBucket[key] = point
		}
		point.Value += event.Value
		point.Count++
	}
	out := make([]core.MetricPoint, 0, len(byBucket))
	for _, point := range byBucket {
		out = append(out, *point)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bucket.Before(out[j].Bucket) })
	return out, nil
}

// ListDimensions returns known dimension and tag values.
func (s *Store) ListDimensions(ctx context.Context) (core.MetricDimensions, error) {
	events, err := s.scanEvents(ctx, core.MetricFilter{From: time.Now().UTC().Add(-30 * 24 * time.Hour), To: time.Now().UTC()})
	if err != nil {
		return core.MetricDimensions{}, err
	}
	sets := map[string]map[string]struct{}{
		"segments":  {},
		"workflows": {},
		"steps":     {},
		"statuses":  {},
		"sources":   {},
	}
	tagSets := map[string]map[string]struct{}{}
	for _, record := range events {
		event := record.Event
		add(sets["segments"], event.Segment)
		add(sets["workflows"], event.Workflow)
		add(sets["steps"], event.Step)
		add(sets["statuses"], event.Status)
		add(sets["sources"], event.Source)
		for key, value := range event.Tags {
			if tagSets[key] == nil {
				tagSets[key] = map[string]struct{}{}
			}
			add(tagSets[key], value)
		}
	}
	tags := map[string][]string{}
	tagKeys := make([]string, 0, len(tagSets))
	for key, set := range tagSets {
		tagKeys = append(tagKeys, key)
		tags[key] = sortedValues(set)
	}
	sort.Strings(tagKeys)
	return core.MetricDimensions{
		Segments:  sortedValues(sets["segments"]),
		Workflows: sortedValues(sets["workflows"]),
		Steps:     sortedValues(sets["steps"]),
		Statuses:  sortedValues(sets["statuses"]),
		Sources:   sortedValues(sets["sources"]),
		TagKeys:   tagKeys,
		Tags:      tags,
	}, nil
}

// ListDashboards returns all dashboard definitions.
func (s *Store) ListDashboards(_ context.Context) ([]core.Dashboard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.Dashboard, 0, len(s.dashboards))
	for _, dashboard := range s.dashboards {
		out = append(out, dashboard)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SaveDashboard creates or replaces a dashboard definition.
func (s *Store) SaveDashboard(_ context.Context, dashboard core.Dashboard) (core.Dashboard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dashboards[dashboard.ID] = dashboard
	return dashboard, nil
}

// DeleteDashboard removes a dashboard definition.
func (s *Store) DeleteDashboard(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.dashboards, id)
	return nil
}

func (s *Store) scanEvents(ctx context.Context, filter core.MetricFilter) ([]core.MetricEventRecord, error) {
	if filter.Tags["trace_id"] != "" {
		return s.queryTraceEvents(ctx, filter)
	}
	if filter.Tags["correlation_id"] != "" {
		return s.queryCorrelationEvents(ctx, filter)
	}
	now := time.Now().UTC()
	out := []core.MetricEventRecord{}
	var startKey map[string]types.AttributeValue
	for {
		resp, err := s.client.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(s.table), ExclusiveStartKey: startKey})
		if err != nil {
			return nil, err
		}
		for _, item := range resp.Items {
			var stored storedMetric
			if err := attributevalue.UnmarshalMap(item, &stored); err != nil {
				return nil, err
			}
			if stored.ExpiresAt > 0 && stored.ExpiresAt <= now.Unix() {
				continue
			}
			var event core.MetricEvent
			if err := json.Unmarshal([]byte(stored.EventJSON), &event); err != nil {
				return nil, err
			}
			if !matches(event, filter) {
				continue
			}
			out = append(out, core.MetricEventRecord{ID: stored.ID, Event: event, ExpiresAt: time.Unix(stored.ExpiresAt, 0).UTC()})
		}
		if len(resp.LastEvaluatedKey) == 0 {
			break
		}
		startKey = resp.LastEvaluatedKey
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Event.Timestamp.Before(out[j].Event.Timestamp) })
	return out, nil
}

func (s *Store) queryCorrelationEvents(ctx context.Context, filter core.MetricFilter) ([]core.MetricEventRecord, error) {
	return s.queryIndexedEvents(ctx, filter, "correlation-index", "correlation_id", filter.Tags["correlation_id"])
}

func (s *Store) queryTraceEvents(ctx context.Context, filter core.MetricFilter) ([]core.MetricEventRecord, error) {
	return s.queryIndexedEvents(ctx, filter, "trace-index", "trace_id", filter.Tags["trace_id"])
}

func (s *Store) queryIndexedEvents(ctx context.Context, filter core.MetricFilter, indexName, keyName, keyValue string) ([]core.MetricEventRecord, error) {
	now := time.Now().UTC()
	out := []core.MetricEventRecord{}
	var startKey map[string]types.AttributeValue
	for {
		resp, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			IndexName:              aws.String(indexName),
			KeyConditionExpression: aws.String(keyName + " = :value"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":value": &types.AttributeValueMemberS{Value: keyValue},
			},
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range resp.Items {
			var stored storedMetric
			if err := attributevalue.UnmarshalMap(item, &stored); err != nil {
				return nil, err
			}
			if stored.ExpiresAt > 0 && stored.ExpiresAt <= now.Unix() {
				continue
			}
			var event core.MetricEvent
			if err := json.Unmarshal([]byte(stored.EventJSON), &event); err != nil {
				return nil, err
			}
			if !matches(event, filter) {
				continue
			}
			out = append(out, core.MetricEventRecord{ID: stored.ID, Event: event, ExpiresAt: time.Unix(stored.ExpiresAt, 0).UTC()})
		}
		if len(resp.LastEvaluatedKey) == 0 {
			break
		}
		startKey = resp.LastEvaluatedKey
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Event.Timestamp.Before(out[j].Event.Timestamp) })
	return out, nil
}
