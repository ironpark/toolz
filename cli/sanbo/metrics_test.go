package main

import (
	"strings"
	"testing"
	"time"
)

func TestMetricsHistogramsRenderCumulativeBuckets(t *testing.T) {
	relay := &Relay{}
	relay.observeDeliveryWait(500 * time.Microsecond)
	relay.observeDeliveryWait(20 * time.Millisecond)
	relay.observeDeliveryWait(11 * time.Second)
	relay.observeFrame(512)
	relay.observeFrame(65 * 1024)
	relay.observeFrame(9 * 1024 * 1024)

	var metrics strings.Builder
	relay.renderHistograms(&metrics)
	requireMetricLines(t, metrics.String(),
		"# HELP paseo_relay_delivery_wait_seconds Time a source waits for synchronous downstream delivery.\n",
		"# TYPE paseo_relay_delivery_wait_seconds histogram\n",
		"paseo_relay_delivery_wait_seconds_bucket{le=\"0.001\"} 1\n",
		"paseo_relay_delivery_wait_seconds_bucket{le=\"0.01\"} 1\n",
		"paseo_relay_delivery_wait_seconds_bucket{le=\"0.1\"} 2\n",
		"paseo_relay_delivery_wait_seconds_bucket{le=\"1\"} 2\n",
		"paseo_relay_delivery_wait_seconds_bucket{le=\"10\"} 2\n",
		"paseo_relay_delivery_wait_seconds_bucket{le=\"+Inf\"} 3\n",
		"paseo_relay_delivery_wait_seconds_sum 11.0205\n",
		"paseo_relay_delivery_wait_seconds_count 3\n",
		"# HELP paseo_relay_frame_size_bytes WebSocket payload-size distribution.\n",
		"# TYPE paseo_relay_frame_size_bytes histogram\n",
		"paseo_relay_frame_size_bytes_bucket{le=\"1024\"} 1\n",
		"paseo_relay_frame_size_bytes_bucket{le=\"65536\"} 1\n",
		"paseo_relay_frame_size_bytes_bucket{le=\"1048576\"} 2\n",
		"paseo_relay_frame_size_bytes_bucket{le=\"8388608\"} 2\n",
		"paseo_relay_frame_size_bytes_bucket{le=\"33554418\"} 3\n",
		"paseo_relay_frame_size_bytes_bucket{le=\"+Inf\"} 3\n",
		"paseo_relay_frame_size_bytes_sum 9504256\n",
		"paseo_relay_frame_size_bytes_count 3\n",
	)
}
