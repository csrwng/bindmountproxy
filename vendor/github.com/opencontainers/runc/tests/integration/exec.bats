#!/usr/bin/env bats

load helpers

function setup() {
  teardown_busybox
  setup_busybox
}

function teardown() {
  teardown_busybox
}

@test "runc exec" {
  # start busybox detached
  run "$RUNC" start -d --console /dev/pts/ptmx test_busybox
  [ "$status" -eq 0 ]

  wait_for_container 15 1 test_busybox

  run "$RUNC" exec test_busybox echo Hello from exec 
  [ "$status" -eq 0 ]
  echo text echoed = "'""${output}""'" 
  [[ "${output}" == *"Hello from exec"* ]]
}

@test "runc exec --pid-file" {
  # start busybox detached
  run "$RUNC" start -d --console /dev/pts/ptmx test_busybox
  [ "$status" -eq 0 ]

  wait_for_container 15 1 test_busybox

  run "$RUNC" exec --pid-file pid.txt test_busybox echo Hello from exec 
  [ "$status" -eq 0 ]
  echo text echoed = "'""${output}""'" 
  [[ "${output}" == *"Hello from exec"* ]]

  # check pid.txt was generated 
  [ -e pid.txt ]

  run cat pid.txt
  [ "$status" -eq 0 ]
  [[ ${lines[0]} =~ [0-9]+ ]]
}
