#!/usr/bin/env bats

load helpers

UPDATE_TEST_RUNC_ROOT="$BATS_TMPDIR/runc-update-integration-test"

CGROUP_MEMORY=""
CGROUP_CPUSET=""
CGROUP_CPU=""
CGROUP_BLKIO=""

function init_cgroup_path() {
    for g in MEMORY CPUSET CPU BLKIO; do
        base_path=$(grep "rw,"  /proc/self/mountinfo | grep -i -m 1 "$g\$" | cut -d ' ' -f 5)
        eval CGROUP_${g}="${base_path}/runc-update-integration-test"
    done
}

function teardown() {
    rm -f $BATS_TMPDIR/runc-update-integration-test.json
    teardown_running_container_inroot test_update $UPDATE_TEST_RUNC_ROOT
    teardown_busybox
}

function setup() {
    teardown
    setup_busybox

    # Add cgroup path
    sed -i 's/\("linux": {\)/\1\n    "cgroupsPath": "runc-update-integration-test",/'  ${BUSYBOX_BUNDLE}/config.json

    # Set some initial known values
    DATA=$(cat <<EOF
    "memory": {
        "limit": 33554432,
        "reservation": 25165824,
        "kernel": 16777216,
        "kernelTCP": 11534336
    },
    "cpu": {
        "shares": 100,
        "quota": 500000,
        "period": 1000000,
        "cpus": "0"
    },
    "blockio": {
        "blkioWeight": 1000
    },
EOF
    )
    DATA=$(echo ${DATA} | sed 's/\n/\\n/g')
    sed -i "s/\(\"resources\": {\)/\1\n${DATA}/" ${BUSYBOX_BUNDLE}/config.json
}

function check_cgroup_value() {
    cgroup=$1
    source=$2
    expected=$3

    current=$(cat $cgroup/$source)
    [ "$current" -eq "$expected" ]
}

@test "update" {
  # start a few busyboxes detached
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT start -d --console /dev/pts/ptmx test_update
    [ "$status" -eq 0 ]
    wait_for_container_inroot 15 1 test_update $UPDATE_TEST_RUNC_ROOT

    init_cgroup_path

    # check that initial values were properly set
    check_cgroup_value $CGROUP_BLKIO "blkio.weight" 1000
    check_cgroup_value $CGROUP_CPU "cpu.cfs_period_us" 1000000
    check_cgroup_value $CGROUP_CPU "cpu.cfs_quota_us" 500000
    check_cgroup_value $CGROUP_CPU "cpu.shares" 100
    check_cgroup_value $CGROUP_CPUSET "cpuset.cpus" 0
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.limit_in_bytes" 16777216
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.tcp.limit_in_bytes" 11534336
    check_cgroup_value $CGROUP_MEMORY "memory.limit_in_bytes" 33554432
    check_cgroup_value $CGROUP_MEMORY "memory.soft_limit_in_bytes" 25165824

    # update blkio-weight
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --blkio-weight 500
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_BLKIO "blkio.weight" 500

    # update cpu-period
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --cpu-period 900000
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_CPU "cpu.cfs_period_us" 900000

    # update cpu-quota
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --cpu-quota 600000
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_CPU "cpu.cfs_quota_us" 600000

    # update cpu-shares
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --cpu-share 200
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_CPU "cpu.shares" 200

    # update cpuset if supported (i.e. we're running on a multicore cpu)
    cpu_count=$(grep '^processor' /proc/cpuinfo | wc -l)
    if [ $cpu_count -ge 1 ]; then
        run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --cpuset-cpus "1"
        [ "$status" -eq 0 ]
        check_cgroup_value $CGROUP_CPUSET "cpuset.cpus" 1
    fi

    # update memory limit
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --memory 67108864
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_MEMORY "memory.limit_in_bytes" 67108864

    # update memory soft limit
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --memory-reservation 33554432
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_MEMORY "memory.soft_limit_in_bytes" 33554432

    # update memory swap (if available)
    if [ -f "$CGROUP_MEMORY/memory.memsw.limit_in_bytes" ]; then
        run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --memory-swap 96468992
        [ "$status" -eq 0 ]
        check_cgroup_value $CGROUP_MEMORY "memory.memsw.limit_in_bytes" 96468992
    fi

    # update kernel memory limit
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --kernel-memory 50331648
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.limit_in_bytes" 50331648

    # update kernel memory tcp limit
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --kernel-memory-tcp 41943040
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.tcp.limit_in_bytes" 41943040

    # Revert to the test initial value via json on stding
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update  -r - test_update <<EOF
{
  "memory": {
    "limit": 33554432,
    "reservation": 25165824,
    "kernel": 16777216,
    "kernelTCP": 11534336
  },
  "cpu": {
    "shares": 100,
    "quota": 500000,
    "period": 1000000,
    "cpus": "0"
  },
  "blockIO": {
    "blkioWeight": 1000
  }
}
EOF
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_BLKIO "blkio.weight" 1000
    check_cgroup_value $CGROUP_CPU "cpu.cfs_period_us" 1000000
    check_cgroup_value $CGROUP_CPU "cpu.cfs_quota_us" 500000
    check_cgroup_value $CGROUP_CPU "cpu.shares" 100
    check_cgroup_value $CGROUP_CPUSET "cpuset.cpus" 0
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.limit_in_bytes" 16777216
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.tcp.limit_in_bytes" 11534336
    check_cgroup_value $CGROUP_MEMORY "memory.limit_in_bytes" 33554432
    check_cgroup_value $CGROUP_MEMORY "memory.soft_limit_in_bytes" 25165824

    # redo all the changes at once
    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update test_update --blkio-weight 500 \
        --cpu-period 900000 --cpu-quota 600000 --cpu-share 200 --memory 67108864 \
        --memory-reservation 33554432 --kernel-memory 50331648 --kernel-memory-tcp 41943040
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_BLKIO "blkio.weight" 500
    check_cgroup_value $CGROUP_CPU "cpu.cfs_period_us" 900000
    check_cgroup_value $CGROUP_CPU "cpu.cfs_quota_us" 600000
    check_cgroup_value $CGROUP_CPU "cpu.shares" 200
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.limit_in_bytes" 50331648
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.tcp.limit_in_bytes" 41943040
    check_cgroup_value $CGROUP_MEMORY "memory.limit_in_bytes" 67108864
    check_cgroup_value $CGROUP_MEMORY "memory.soft_limit_in_bytes" 33554432

    # reset to initial test value via json file
    DATA=$(cat <<"EOF"
{
  "memory": {
    "limit": 33554432,
    "reservation": 25165824,
    "kernel": 16777216,
    "kernelTCP": 11534336
  },
  "cpu": {
    "shares": 100,
    "quota": 500000,
    "period": 1000000,
    "cpus": "0"
  },
  "blockIO": {
    "blkioWeight": 1000
  }
}
EOF
)
    echo $DATA > $BATS_TMPDIR/runc-update-integration-test.json

    run "$RUNC" --root $UPDATE_TEST_RUNC_ROOT update  -r $BATS_TMPDIR/runc-update-integration-test.json test_update
    [ "$status" -eq 0 ]
    check_cgroup_value $CGROUP_BLKIO "blkio.weight" 1000
    check_cgroup_value $CGROUP_CPU "cpu.cfs_period_us" 1000000
    check_cgroup_value $CGROUP_CPU "cpu.cfs_quota_us" 500000
    check_cgroup_value $CGROUP_CPU "cpu.shares" 100
    check_cgroup_value $CGROUP_CPUSET "cpuset.cpus" 0
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.limit_in_bytes" 16777216
    check_cgroup_value $CGROUP_MEMORY "memory.kmem.tcp.limit_in_bytes" 11534336
    check_cgroup_value $CGROUP_MEMORY "memory.limit_in_bytes" 33554432
    check_cgroup_value $CGROUP_MEMORY "memory.soft_limit_in_bytes" 25165824
}
