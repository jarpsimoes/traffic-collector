// SPDX-License-Identifier: GPL-2.0 OR BSD-3-Clause

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

/* Traffic event structure */
struct traffic_event {
    __u64 timestamp;
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u8  protocol;
    __u8  _pad[3];
    __u32 pid;
    __u32 tid;
    __u64 bytes_sent;
};

/* Ring buffer for events */
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

/* Connection tracking */
struct connection_key {
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
};

struct connection_info {
    __u64 packets_sent;
    __u64 bytes_sent;
    __u64 packets_recv;
    __u64 bytes_recv;
    __u64 start_time;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, struct connection_key);
    __type(value, struct connection_info);
} connections SEC(".maps");

/* TCP events */
int trace_tcp_sendmsg(struct pt_regs *ctx, struct sock *sk, struct msghdr *msg, size_t size) {
    struct traffic_event *event;
    struct connection_key conn_key = {};
    struct connection_info *conn_info;
    __u64 pid_tgid;
    __u16 family;

    // Get current PID/TID
    pid_tgid = bpf_get_current_pid_tgid();

    // Get socket info
    __u16 sport = sk->__sk_common.skc_num;
    __u32 saddr = sk->__sk_common.skc_rcv_saddr;
    __u32 daddr = sk->__sk_common.skc_daddr;
    __u16 dport = bpf_ntohs(sk->__sk_common.skc_dport);

    // Reserve space in ring buffer
    event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
    if (!event)
        return 0;

    // Fill event
    event->timestamp = bpf_ktime_get_ns();
    event->saddr = saddr;
    event->daddr = daddr;
    event->sport = sport;
    event->dport = dport;
    event->protocol = IPPROTO_TCP;
    event->pid = pid_tgid >> 32;
    event->tid = (__u32)pid_tgid;
    event->bytes_sent = size;

    // Submit event
    bpf_ringbuf_submit(event, 0);

    // Update connection tracking
    conn_key.saddr = saddr;
    conn_key.daddr = daddr;
    conn_key.sport = sport;
    conn_key.dport = dport;

    conn_info = bpf_map_lookup_elem(&connections, &conn_key);
    if (conn_info) {
        __sync_fetch_and_add(&conn_info->packets_sent, 1);
        __sync_fetch_and_add(&conn_info->bytes_sent, size);
    } else {
        struct connection_info new_conn = {
            .packets_sent = 1,
            .bytes_sent = size,
            .packets_recv = 0,
            .bytes_recv = 0,
            .start_time = event->timestamp,
        };
        bpf_map_update_elem(&connections, &conn_key, &new_conn, 0);
    }

    return 0;
}

/* UDP events */
int trace_udp_sendmsg(struct pt_regs *ctx, struct sock *sk, struct msghdr *msg, size_t len) {
    struct traffic_event *event;
    __u64 pid_tgid;

    pid_tgid = bpf_get_current_pid_tgid();

    __u16 sport = sk->__sk_common.skc_num;
    __u32 saddr = sk->__sk_common.skc_rcv_saddr;
    __u32 daddr = sk->__sk_common.skc_daddr;
    __u16 dport = bpf_ntohs(sk->__sk_common.skc_dport);

    event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
    if (!event)
        return 0;

    event->timestamp = bpf_ktime_get_ns();
    event->saddr = saddr;
    event->daddr = daddr;
    event->sport = sport;
    event->dport = dport;
    event->protocol = IPPROTO_UDP;
    event->pid = pid_tgid >> 32;
    event->tid = (__u32)pid_tgid;
    event->bytes_sent = len;

    bpf_ringbuf_submit(event, 0);

    return 0;
}

/* Syscall tracepoint for network events */
TRACEPOINT_PROBE(syscalls, sys_enter_write) {
    // Track write syscalls for additional network visibility
    return 0;
}
