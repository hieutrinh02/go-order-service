package metrics

import "github.com/prometheus/client_golang/prometheus"

var HTTPRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "order_service_http_requests_total",
		Help: "Total number of HTTP requests.",
	},
	[]string{"method", "path", "status"},
)

var HTTPRequestDurationSeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "order_service_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"method", "path", "status"},
)

var OrdersCreatedTotal = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "order_service_orders_created_total",
		Help: "Total number of orders created.",
	},
)

var PaymentsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "order_service_payments_total",
		Help: "Total number of payment attempts.",
	},
	[]string{"status"},
)

var OutboxEventsPublishedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "order_service_outbox_events_published_total",
		Help: "Total number of outbox events published.",
	},
	[]string{"event_type"},
)

var OutboxEventsFailedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "order_service_outbox_events_failed_total",
		Help: "Total number of outbox event publish failures.",
	},
	[]string{"event_type"},
)

var ConsumerEventsProcessedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "order_service_consumer_events_processed_total",
		Help: "Total number of consumer events processed.",
	},
	[]string{"event_type"},
)

var ConsumerEventsDuplicateTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "order_service_consumer_events_duplicate_total",
		Help: "Total number of duplicate consumer events skipped.",
	},
	[]string{"event_type"},
)

var RateLimitAllowedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "order_service_rate_limit_allowed_total",
		Help: "Total number of requests allowed by the Redis rate limiter.",
	},
	[]string{"scope"},
)

var RateLimitBlockedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "order_service_rate_limit_blocked_total",
		Help: "Total number of requests blocked by the Redis rate limiter.",
	},
	[]string{"scope"},
)

func Register() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDurationSeconds,
		OrdersCreatedTotal,
		PaymentsTotal,
		OutboxEventsPublishedTotal,
		OutboxEventsFailedTotal,
		ConsumerEventsProcessedTotal,
		ConsumerEventsDuplicateTotal,
		RateLimitAllowedTotal,
		RateLimitBlockedTotal,
	)
}
