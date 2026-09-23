$ErrorActionPreference = "Stop"

$projectRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Push-Location $projectRoot

try {
    function Invoke-AjvCase {
        param(
            [Parameter(Mandatory = $true)][string]$Schema,
            [Parameter(Mandatory = $true)][string[]]$Data,
            [Parameter(Mandatory = $true)][int]$ExpectedExit,
            [string[]]$References = @()
        )

        $ajvArguments = @("validate", "--spec=draft2020", "-c", "ajv-formats", "-s", $Schema)
        foreach ($dataPath in $Data) {
            $ajvArguments += @("-d", $dataPath)
        }
        foreach ($reference in $References) {
            $ajvArguments += @("-r", $reference)
        }

        & npx --yes --package ajv-cli@5 --package ajv-formats@2 ajv @ajvArguments
        $actualExit = $LASTEXITCODE
        if ($actualExit -ne $ExpectedExit) {
            throw "Ajv case failed for $($Data -join ', ')`: expected exit $ExpectedExit, got $actualExit"
        }
    }

    function Invoke-WorkerSemanticCase {
        param(
            [Parameter(Mandatory = $true)][string]$Kind,
            [Parameter(Mandatory = $true)][string]$Data,
            [Parameter(Mandatory = $true)][int]$ExpectedExit,
            [string[]]$ExtraArguments = @()
        )

        & node "scripts/validate-worker-semantics.mjs" $Kind $Data @ExtraArguments
        $actualExit = $LASTEXITCODE
        if ($actualExit -ne $ExpectedExit) {
            throw "Worker semantic case failed for $Data`: expected exit $ExpectedExit, got $actualExit"
        }
    }

    function Invoke-ProtocolValidatorCase {
        param(
            [Parameter(Mandatory = $true)][string]$Kind,
            [Parameter(Mandatory = $true)][string]$Data,
            [Parameter(Mandatory = $true)][int]$ExpectedExit,
            [string]$Request = ""
        )

        $caseProjectRoot = $projectRoot
        if ($Kind -in @("memory-index", "memory-request", "memory-response", "memory-source")) {
            $caseProjectRoot = $validatorFixtureRoot
        }
        $validatorArguments = @(
            "xiaotao/scripts/validate.py",
            $Kind,
            $Data,
            "--project-root",
            $caseProjectRoot
        )
        if ($Kind -eq "memory-response") {
            if ([string]::IsNullOrEmpty($Request)) {
                $Request = "$validatorFixtureRoot/memory-request-valid.json"
            }
            $validatorArguments += @("--request", $Request)
        }

        & python @validatorArguments
        $actualExit = $LASTEXITCODE
        if ($actualExit -ne $ExpectedExit) {
            throw "Protocol validator case failed for $Data`: expected exit $ExpectedExit, got $actualExit"
        }
    }

    function Invoke-ProtocolDiagnosticCase {
        param(
            [Parameter(Mandatory = $true)][string]$Kind,
            [Parameter(Mandatory = $true)][string]$Data,
            [Parameter(Mandatory = $true)][string]$ExpectedPath,
            [Parameter(Mandatory = $true)][string]$ExpectedMessage,
            [string]$Request = ""
        )

        $caseProjectRoot = $projectRoot
        if ($Kind -in @("memory-index", "memory-request", "memory-response", "memory-source")) {
            $caseProjectRoot = $validatorFixtureRoot
        }
        $validatorArguments = @(
            "xiaotao/scripts/validate.py",
            $Kind,
            $Data,
            "--project-root",
            $caseProjectRoot,
            "--json"
        )
        if ($Kind -eq "memory-response") {
            if ([string]::IsNullOrEmpty($Request)) {
                $Request = "$validatorFixtureRoot/memory-request-valid.json"
            }
            $validatorArguments += @("--request", $Request)
        }

        $rawResult = & python @validatorArguments
        $actualExit = $LASTEXITCODE
        if ($actualExit -ne 1) {
            throw "Protocol diagnostic case failed for $Data`: expected exit 1, got $actualExit"
        }

        $result = $rawResult | ConvertFrom-Json
        $matchingErrors = @($result.errors | Where-Object {
            $_.path -eq $ExpectedPath -and $_.message -like "*$ExpectedMessage*"
        })
        if ($result.valid -ne $false -or $matchingErrors.Count -eq 0) {
            throw "Protocol diagnostic case returned no matching diagnostic for $Data"
        }
    }

    function Invoke-ProtocolSchemaParityCase {
        param(
            [Parameter(Mandatory = $true)][string]$Schema,
            [Parameter(Mandatory = $true)][string]$Kind,
            [Parameter(Mandatory = $true)][string]$Data,
            [Parameter(Mandatory = $true)][int]$ExpectedExit,
            [string]$Request = ""
        )

        Invoke-AjvCase $Schema $Data $ExpectedExit
        Invoke-ProtocolValidatorCase $Kind $Data $ExpectedExit $Request
    }

    Get-ChildItem "xiaotao/references/schemas" -Filter "*.json" | ForEach-Object {
        Get-Content -Raw $_.FullName | ConvertFrom-Json | Out-Null
    }
    Get-ChildItem "xiaotao/references/scenarios/schema-fixtures" -Filter "*.json" | ForEach-Object {
        Get-Content -Raw $_.FullName | ConvertFrom-Json | Out-Null
    }

    $validatorFixtureRoot = "xiaotao/references/scenarios/validator-fixtures"
    $handoffSchema = "xiaotao/references/schemas/handoff.schema.json"
    $memoryIndexSchema = "xiaotao/references/schemas/memory-index.schema.json"
    $memoryRequestSchema = "xiaotao/references/schemas/memory-worker-request.schema.json"
    $memoryResponseSchema = "xiaotao/references/schemas/memory-worker-response.schema.json"
    $memorySourceSchema = "xiaotao/references/schemas/memory-source.schema.json"
    $memoryMergeRequestSchema = "xiaotao/references/schemas/memory-merge-request.schema.json"
    $memoryMergeResponseSchema = "xiaotao/references/schemas/memory-merge-response.schema.json"

    & python "xiaotao/scripts/validate.py" memory-response `
        "$validatorFixtureRoot/memory-response-valid.json" `
        --project-root $validatorFixtureRoot 2>$null
    if ($LASTEXITCODE -ne 2) {
        throw "memory-response validation must require an external --request"
    }

    Invoke-ProtocolSchemaParityCase $handoffSchema "handoff" `
        "$validatorFixtureRoot/handoff-valid.json" 0
    Invoke-ProtocolSchemaParityCase $handoffSchema "handoff" `
        "$validatorFixtureRoot/handoff-schema-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $handoffSchema "handoff" `
        "$validatorFixtureRoot/handoff-legacy-role-invalid.json" 1
    Invoke-ProtocolDiagnosticCase "handoff" `
        "$validatorFixtureRoot/handoff-legacy-role-invalid.json" `
        '$.role_state_path' "legacy role_state_path field is no longer supported"
    Invoke-ProtocolDiagnosticCase "handoff" `
        "$validatorFixtureRoot/handoff-legacy-role-invalid.json" `
        '$.recommended_next[0].role' "legacy role field is no longer supported"
    Invoke-ProtocolSchemaParityCase $memoryIndexSchema "memory-index" `
        "$validatorFixtureRoot/memory-index-valid.json" 0
    Invoke-ProtocolSchemaParityCase $memoryIndexSchema "memory-index" `
        "$validatorFixtureRoot/memory-index-cjk-valid.json" 0
    Invoke-ProtocolSchemaParityCase $memoryIndexSchema "memory-index" `
        "$validatorFixtureRoot/memory-index-storage-id-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryIndexSchema "memory-index" `
        "$validatorFixtureRoot/memory-index-schema-invalid.json" 1
    Invoke-AjvCase $memoryIndexSchema `
        "$validatorFixtureRoot/memory-index-duplicate-id-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-index" `
        "$validatorFixtureRoot/memory-index-duplicate-id-invalid.json" `
        '$.entries[1].memory_id' "must be unique"
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-valid.json" 0
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-schema-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-decision-context-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-decision-context-missing-reason-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-playbook-revision-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-playbook-path-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-playbook-reserved-path-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-empty-playbooks-valid.json" 0
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-bounded-valid.json" 0
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-bounded-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memorySourceSchema "memory-source" `
        "$validatorFixtureRoot/.xiaotao/memory/sources/src-8f10f07960b6b8d5.json" 0
    Invoke-AjvCase $memorySourceSchema `
        "$validatorFixtureRoot/.xiaotao/memory/sources/src-1111111111111111.json" 0
    Invoke-ProtocolDiagnosticCase "memory-source" `
        "$validatorFixtureRoot/.xiaotao/memory/sources/src-1111111111111111.json" `
        '$.content_sha256' "must match"
    Invoke-ProtocolSchemaParityCase $memoryRequestSchema "memory-request" `
        "$validatorFixtureRoot/memory-request-explicit-chat-valid.json" 0
    Invoke-ProtocolSchemaParityCase $memoryResponseSchema "memory-response" `
        "$validatorFixtureRoot/memory-response-explicit-chat-valid.json" 0 `
        "$validatorFixtureRoot/memory-request-explicit-chat-valid.json"
    Invoke-ProtocolSchemaParityCase $memoryResponseSchema "memory-response" `
        "$validatorFixtureRoot/memory-response-valid.json" 0
    Invoke-ProtocolSchemaParityCase $memoryResponseSchema "memory-response" `
        "$validatorFixtureRoot/memory-response-schema-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryResponseSchema "memory-response" `
        "$validatorFixtureRoot/memory-response-action-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryResponseSchema "memory-response" `
        "$validatorFixtureRoot/memory-response-conflict-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryResponseSchema "memory-response" `
        "$validatorFixtureRoot/memory-response-date-time-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryResponseSchema "memory-response" `
        "$validatorFixtureRoot/memory-response-playbook-action-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryResponseSchema "memory-response" `
        "$validatorFixtureRoot/memory-response-playbook-status-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryResponseSchema "memory-response" `
        "$validatorFixtureRoot/memory-response-playbook-evidence-required-invalid.json" 1
    Invoke-AjvCase $memoryResponseSchema `
        "$validatorFixtureRoot/memory-response-bounded-target-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-response" `
        "$validatorFixtureRoot/memory-response-bounded-target-invalid.json" `
        '$.long_term_candidates[0].match.entry_ids[0]' "externally supplied request" `
        "$validatorFixtureRoot/memory-request-bounded-valid.json"
    Invoke-ProtocolSchemaParityCase $memoryMergeRequestSchema "memory-merge-request" `
        "$validatorFixtureRoot/memory-merge-request-valid.json" 0
    Invoke-ProtocolSchemaParityCase $memoryMergeRequestSchema "memory-merge-request" `
        "$validatorFixtureRoot/memory-merge-request-schema-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryMergeResponseSchema "memory-merge-response" `
        "$validatorFixtureRoot/memory-merge-response-valid.json" 0
    Invoke-ProtocolSchemaParityCase $memoryMergeResponseSchema "memory-merge-response" `
        "$validatorFixtureRoot/memory-merge-response-schema-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryMergeResponseSchema "memory-merge-response" `
        "$validatorFixtureRoot/memory-merge-response-conflict-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $memoryMergeResponseSchema "memory-merge-response" `
        "$validatorFixtureRoot/memory-merge-response-date-time-invalid.json" 1

    Invoke-ProtocolValidatorCase "handoff" `
        "$validatorFixtureRoot/handoff-traversal-invalid.json" 1
    Invoke-ProtocolValidatorCase "handoff" "$validatorFixtureRoot/invalid-json.json" 1
    Invoke-ProtocolValidatorCase "memory-request" `
        "$validatorFixtureRoot/memory-request-missing-reference-invalid.json" 1
    Invoke-ProtocolValidatorCase "memory-merge-request" `
        "$validatorFixtureRoot/memory-merge-request-missing-reference-invalid.json" 1
    Invoke-ProtocolValidatorCase "memory-merge-response" `
        "$validatorFixtureRoot/memory-merge-response-missing-reference-invalid.json" 1
    Invoke-AjvCase $memoryMergeResponseSchema `
        "$validatorFixtureRoot/memory-merge-response-duplicate-id-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-merge-response" `
        "$validatorFixtureRoot/memory-merge-response-duplicate-id-invalid.json" `
        '$.merged_entries[1].entry_id' "must be unique"
    Invoke-ProtocolDiagnosticCase "memory-merge-response" `
        "$validatorFixtureRoot/memory-merge-response-duplicate-id-invalid.json" `
        '$.unresolved_conflicts[1].conflict_id' "must be unique"
    Invoke-AjvCase $memoryRequestSchema `
        "$validatorFixtureRoot/memory-request-duplicate-id-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-request" `
        "$validatorFixtureRoot/memory-request-duplicate-id-invalid.json" `
        '$.current_memory.long_term_entries[1].entry_id' "must be unique"
    Invoke-AjvCase $memoryRequestSchema `
        "$validatorFixtureRoot/memory-request-playbook-duplicate-id-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-request" `
        "$validatorFixtureRoot/memory-request-playbook-duplicate-id-invalid.json" `
        '$.current_playbooks[1].playbook_id' "must be unique"
    Invoke-AjvCase $memoryRequestSchema `
        "$validatorFixtureRoot/memory-request-playbook-metadata-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-request" `
        "$validatorFixtureRoot/memory-request-playbook-metadata-invalid.json" `
        '$.current_playbooks[0].revision' "must match canonical Playbook metadata"
    Invoke-ProtocolValidatorCase "memory-response" `
        "$validatorFixtureRoot/memory-response-missing-reference-invalid.json" 1
    Invoke-AjvCase $memoryResponseSchema `
        "$validatorFixtureRoot/memory-response-duplicate-id-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-response" `
        "$validatorFixtureRoot/memory-response-duplicate-id-invalid.json" `
        '$.long_term_candidates[1].candidate_id' "must be unique"
    Invoke-AjvCase $memoryResponseSchema `
        "$validatorFixtureRoot/memory-response-playbook-duplicate-id-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-response" `
        "$validatorFixtureRoot/memory-response-playbook-duplicate-id-invalid.json" `
        '$.playbook_candidates[1].candidate_id' "must be unique"
    Invoke-ProtocolValidatorCase "memory-response" `
        "$validatorFixtureRoot/memory-response-playbook-missing-reference-invalid.json" 1
    Invoke-AjvCase $memoryResponseSchema `
        "$validatorFixtureRoot/memory-response-playbook-unknown-target-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-response" `
        "$validatorFixtureRoot/memory-response-playbook-unknown-target-invalid.json" `
        '$.playbook_candidates[0].match.playbook_ids[0]' "must reference a Playbook from the externally supplied request"
    Invoke-AjvCase $memoryResponseSchema `
        "$validatorFixtureRoot/memory-response-request-mismatch-invalid.json" 0
    Invoke-ProtocolDiagnosticCase "memory-response" `
        "$validatorFixtureRoot/memory-response-request-mismatch-invalid.json" `
        '$.request_file' "must match the externally supplied --request file" `
        "$validatorFixtureRoot/memory-request-empty-playbooks-valid.json"
    Invoke-ProtocolDiagnosticCase "memory-response" `
        "$validatorFixtureRoot/memory-response-request-mismatch-invalid.json" `
        '$.playbook_candidates[0].match.playbook_ids[0]' `
        "must reference a Playbook from the externally supplied request" `
        "$validatorFixtureRoot/memory-request-empty-playbooks-valid.json"
    Invoke-ProtocolValidatorCase "memory-response" `
        "$validatorFixtureRoot/memory-response-request-missing-invalid.json" 1
    Invoke-ProtocolValidatorCase "decision-record" `
        "$validatorFixtureRoot/decision-record-missing-reference-invalid.json" 1
    $activityEventSchema = "xiaotao/references/schemas/activity-event.schema.json"
    Invoke-ProtocolSchemaParityCase $activityEventSchema "activity-event" `
        "$validatorFixtureRoot/activity-event-temporary-promoted-valid.json" 0
    Invoke-ProtocolSchemaParityCase $activityEventSchema "activity-event" `
        "$validatorFixtureRoot/activity-event-unknown-type-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $activityEventSchema "activity-event" `
        "$validatorFixtureRoot/activity-event-checkpoint-recovered-valid.json" 0
    Invoke-ProtocolSchemaParityCase $activityEventSchema "activity-event" `
        "$validatorFixtureRoot/activity-event-worker-approved-valid.json" 0
    $workerApprovalSchema = "xiaotao/references/schemas/worker-approval.schema.json"
    Invoke-ProtocolSchemaParityCase $workerApprovalSchema "worker-approval" `
        "xiaotao/references/scenarios/schema-fixtures/worker-approval-valid.json" 0
    Invoke-ProtocolSchemaParityCase $workerApprovalSchema "worker-approval" `
        "xiaotao/references/scenarios/schema-fixtures/worker-approval-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $workerApprovalSchema "worker-approval" `
        "xiaotao/references/scenarios/schema-fixtures/worker-approval-timezone-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $workerApprovalSchema "worker-approval" `
        "xiaotao/references/scenarios/schema-fixtures/worker-approval-whitespace-invalid.json" 1
    $checkpointObservationSchema = "xiaotao/references/schemas/checkpoint-observation.schema.json"
    Invoke-ProtocolSchemaParityCase $checkpointObservationSchema "checkpoint-observation" `
        "$validatorFixtureRoot/checkpoint-observation-recovery-valid.json" 0
    Invoke-ProtocolSchemaParityCase $checkpointObservationSchema "checkpoint-observation" `
        "$validatorFixtureRoot/checkpoint-observation-partial-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $checkpointObservationSchema "checkpoint-observation" `
        "$validatorFixtureRoot/checkpoint-observation-timezone-invalid.json" 1
    Invoke-ProtocolDiagnosticCase "handoff" `
        "$validatorFixtureRoot/handoff-control-character-invalid.json" `
        '$.result_path' "control character"
    Invoke-ProtocolDiagnosticCase "memory-response" `
        "$validatorFixtureRoot/memory-response-nan-invalid.json" '$' "NaN"
    Invoke-ProtocolDiagnosticCase "memory-response" `
        "$validatorFixtureRoot/memory-response-infinity-invalid.json" '$' "Infinity"
    Invoke-ProtocolDiagnosticCase "memory-response" `
        "$validatorFixtureRoot/memory-response-negative-infinity-invalid.json" '$' "-Infinity"

    $fixtureRoot = "xiaotao/references/scenarios/schema-fixtures"
    Invoke-AjvCase $handoffSchema "$fixtureRoot/handoff-blocked-valid.json" 0
    Invoke-AjvCase $handoffSchema "$fixtureRoot/handoff-completed-valid.json" 0
    Invoke-AjvCase $handoffSchema "$fixtureRoot/handoff-blocked-invalid.json" 1
    Invoke-AjvCase $handoffSchema "$fixtureRoot/handoff-completed-invalid.json" 1
    Invoke-AjvCase $handoffSchema "$fixtureRoot/handoff-completed-with-input-invalid.json" 1
    Invoke-AjvCase "xiaotao/references/schemas/temporary-meta.schema.json" `
        "$fixtureRoot/temporary-meta-valid.json" 0
    Invoke-AjvCase "xiaotao/references/schemas/task.schema.json" "$fixtureRoot/task-valid.json" 0
    Invoke-AjvCase "xiaotao/references/schemas/task.schema.json" `
        "$fixtureRoot/task-completed-valid.json" 0
    $decisionRecordSchema = "xiaotao/references/schemas/decision-record.schema.json"
    Invoke-ProtocolSchemaParityCase $decisionRecordSchema "decision-record" `
        "$fixtureRoot/decision-record-approved-valid.json" 0
    Invoke-ProtocolSchemaParityCase $decisionRecordSchema "decision-record" `
        "$fixtureRoot/decision-record-superseded-valid.json" 0
    Invoke-ProtocolSchemaParityCase $decisionRecordSchema "decision-record" `
        "$fixtureRoot/decision-record-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $decisionRecordSchema "decision-record" `
        "$fixtureRoot/decision-record-superseded-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $decisionRecordSchema "decision-record" `
        "$fixtureRoot/decision-record-rejected-empty-targets-valid.json" 0
    Invoke-ProtocolSchemaParityCase $decisionRecordSchema "decision-record" `
        "$fixtureRoot/decision-record-approved-empty-targets-invalid.json" 1
    Invoke-ProtocolSchemaParityCase $decisionRecordSchema "decision-record" `
        "$fixtureRoot/decision-record-superseded-empty-targets-invalid.json" 1
    $activityEventSchema = "xiaotao/references/schemas/activity-event.schema.json"
    Invoke-ProtocolSchemaParityCase $activityEventSchema "activity-event" `
        "$validatorFixtureRoot/activity-event-playbook-approved-valid.json" 0
    Invoke-ProtocolSchemaParityCase $activityEventSchema "activity-event" `
        "$validatorFixtureRoot/activity-event-playbook-superseded-valid.json" 0
    Invoke-AjvCase "xiaotao/references/schemas/task.schema.json" `
        "$fixtureRoot/task-promoted-invalid.json" 1
    Invoke-AjvCase "xiaotao/references/schemas/task.schema.json" `
        "$fixtureRoot/task-promoted-at-missing-source-invalid.json" 1
    $memoryFollowupSchema = "xiaotao/references/schemas/memory-followup.schema.json"
    Invoke-AjvCase $memoryFollowupSchema "$fixtureRoot/memory-followup-pending-valid.json" 0
    Invoke-AjvCase $memoryFollowupSchema "$fixtureRoot/memory-followup-resolved-valid.json" 0
    Invoke-AjvCase $memoryFollowupSchema `
        "$fixtureRoot/memory-followup-pending-with-resolution-invalid.json" 1
    Invoke-AjvCase $memoryFollowupSchema `
        "$fixtureRoot/memory-followup-resolved-missing-resolution-invalid.json" 1
    Invoke-AjvCase $memoryFollowupSchema `
        "$fixtureRoot/memory-followup-unknown-field-invalid.json" 1
    Invoke-AjvCase $memoryFollowupSchema `
        "$fixtureRoot/memory-followup-duplicate-source-refs-invalid.json" 1
    Invoke-AjvCase $memoryFollowupSchema `
        "$fixtureRoot/memory-followup-duplicate-related-ids-invalid.json" 1
    Invoke-AjvCase $memoryFollowupSchema `
        "$fixtureRoot/memory-followup-duplicate-resolution-refs-invalid.json" 1

    $globalPreferenceSchema = "xiaotao/references/schemas/global-preference.schema.json"
    Invoke-AjvCase $globalPreferenceSchema "$fixtureRoot/global-preference-valid.json" 0
    Invoke-AjvCase $globalPreferenceSchema "$fixtureRoot/global-preference-invalid.json" 1

    $timelineEventSchema = "xiaotao/references/schemas/timeline-event.schema.json"
    Invoke-AjvCase $timelineEventSchema "$fixtureRoot/timeline-event-valid.json" 0
    Invoke-AjvCase $timelineEventSchema "$fixtureRoot/timeline-event-invalid.json" 1

    $workerSchema = "xiaotao/references/schemas/worker.schema.json"
    $requirementsSchema = "xiaotao/references/schemas/capability-requirements.schema.json"
    $registrySchema = "xiaotao/references/schemas/worker-registry.schema.json"
    $selectionSchema = "xiaotao/references/schemas/worker-selection.schema.json"
    $instructionRegistrySchema = "xiaotao/references/schemas/instruction-registry.schema.json"
    $delegationPacketSchema = "xiaotao/references/schemas/delegation-packet.schema.json"
    Invoke-AjvCase $requirementsSchema "$fixtureRoot/capability-requirements-valid.json" 0
    Invoke-WorkerSemanticCase "requirements" "$fixtureRoot/capability-requirements-valid.json" 0
    Invoke-WorkerSemanticCase "requirements" `
        "$fixtureRoot/capability-requirements-overlap-invalid.json" 1
    Invoke-AjvCase $requirementsSchema `
        "$fixtureRoot/capability-requirements-windows-path-invalid.json" 1
    Invoke-AjvCase $workerSchema "$fixtureRoot/worker-temporary-valid.json" 0
    Invoke-AjvCase $workerSchema "$fixtureRoot/worker-temporary-memory-valid.json" 0
    Invoke-AjvCase $workerSchema "$fixtureRoot/worker-session-valid.json" 0
    Invoke-AjvCase $workerSchema "$fixtureRoot/worker-temporary-lifecycle-invalid.json" 1
    Invoke-AjvCase $workerSchema "$fixtureRoot/worker-temporary-memory-lifecycle-invalid.json" 1
    Invoke-AjvCase $workerSchema "$fixtureRoot/worker-session-lifecycle-invalid.json" 1
    Invoke-AjvCase $workerSchema "$fixtureRoot/worker-permission-invalid.json" 1
    Invoke-AjvCase $workerSchema "$fixtureRoot/worker-windows-path-invalid.json" 1
    Invoke-AjvCase $workerSchema "$fixtureRoot/worker-instruction-overlap-invalid.json" 0
    Invoke-WorkerSemanticCase "worker" `
        "$fixtureRoot/worker-instruction-overlap-invalid.json" 1
    Invoke-AjvCase $instructionRegistrySchema `
        "xiaotao/references/instructions/builtin-registry.json" 0
    Invoke-WorkerSemanticCase "instruction-registry" `
        "xiaotao/references/instructions/builtin-registry.json" 0
    Invoke-AjvCase $instructionRegistrySchema `
        "$fixtureRoot/instruction-registry-project-valid.json" 0
    Invoke-WorkerSemanticCase "instruction-registry" `
        "$fixtureRoot/instruction-registry-project-valid.json" 0 `
        @("--builtin", "xiaotao/references/instructions/builtin-registry.json")
    Invoke-AjvCase $instructionRegistrySchema `
        "$fixtureRoot/instruction-registry-duplicate-invalid.json" 0
    Invoke-WorkerSemanticCase "instruction-registry" `
        "$fixtureRoot/instruction-registry-duplicate-invalid.json" 1
    Invoke-AjvCase $instructionRegistrySchema `
        "$fixtureRoot/instruction-registry-override-invalid.json" 0
    Invoke-WorkerSemanticCase "instruction-registry" `
        "$fixtureRoot/instruction-registry-override-invalid.json" 1 `
        @("--builtin", "xiaotao/references/instructions/builtin-registry.json")
    Invoke-AjvCase $workerSchema `
        "$fixtureRoot/worker-delegation-snapshot-valid.json" 0
    $delegationArguments = @(
        "--worker", "$fixtureRoot/worker-delegation-snapshot-valid.json",
        "--builtin", "xiaotao/references/instructions/builtin-registry.json",
        "--core-root", "xiaotao",
        "--project-root", "."
    )
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-supported-valid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-supported-valid.json" 0 $delegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-unsupported-valid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-unsupported-valid.json" 0 $delegationArguments
    Invoke-AjvCase $workerSchema `
        "$fixtureRoot/worker-delegation-unknown-required-snapshot-valid.json" 0
    $unknownInstructionDelegationArguments = @(
        "--worker", "$fixtureRoot/worker-delegation-unknown-required-snapshot-valid.json",
        "--builtin", "xiaotao/references/instructions/builtin-registry.json",
        "--core-root", "xiaotao",
        "--project-root", "."
    )
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-unknown-required-unsupported-valid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-unknown-required-unsupported-valid.json" 0 `
        $unknownInstructionDelegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-unknown-required-unreported-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-unknown-required-unreported-invalid.json" 1 `
        $unknownInstructionDelegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-missing-required-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-missing-required-invalid.json" 1 $delegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-required-refs-mismatch-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-required-refs-mismatch-invalid.json" 1 `
        $delegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-optional-refs-expansion-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-optional-refs-expansion-invalid.json" 1 `
        $delegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-tool-expansion-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-tool-expansion-invalid.json" 1 $delegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-permission-expansion-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-permission-expansion-invalid.json" 1 `
        $delegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-context-expansion-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-context-expansion-invalid.json" 1 $delegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-write-expansion-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-write-expansion-invalid.json" 1 `
        $delegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-digest-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-digest-invalid.json" 1 $delegationArguments
    Invoke-AjvCase $delegationPacketSchema `
        "$fixtureRoot/delegation-packet-source-mismatch-invalid.json" 0
    Invoke-WorkerSemanticCase "delegation" `
        "$fixtureRoot/delegation-packet-source-mismatch-invalid.json" 1 `
        $delegationArguments
    Invoke-AjvCase $registrySchema "xiaotao/references/workers/builtin-registry.json" 0 `
        @($workerSchema)
    Invoke-WorkerSemanticCase "registry" "xiaotao/references/workers/builtin-registry.json" 0
    Invoke-AjvCase $registrySchema "$fixtureRoot/worker-registry-valid.json" 0 @($workerSchema)
    Invoke-WorkerSemanticCase "registry" "$fixtureRoot/worker-registry-valid.json" 0
    Invoke-WorkerSemanticCase "registry" "$fixtureRoot/worker-registry-duplicate-invalid.json" 1
    Invoke-AjvCase $registrySchema "$fixtureRoot/worker-registry-source-invalid.json" 1 `
        @($workerSchema)
    Invoke-AjvCase $selectionSchema "$fixtureRoot/worker-selection-exact-valid.json" 0 `
        @($requirementsSchema)
    Invoke-AjvCase $selectionSchema "$fixtureRoot/worker-selection-composed-valid.json" 0 `
        @($requirementsSchema)
    Invoke-AjvCase $selectionSchema "$fixtureRoot/worker-selection-generated-valid.json" 0 `
        @($requirementsSchema)
    Invoke-AjvCase $selectionSchema "$fixtureRoot/worker-selection-temporary-valid.json" 0 `
        @($requirementsSchema)
    Invoke-AjvCase $selectionSchema "$fixtureRoot/worker-selection-session-valid.json" 0 `
        @($requirementsSchema)
    Invoke-AjvCase $handoffSchema "$fixtureRoot/worker-handoff-valid.json" 0
    Invoke-AjvCase $handoffSchema "$fixtureRoot/worker-handoff-both-paths-invalid.json" 1

    $builtinRegistry = Get-Content -Raw "xiaotao/references/workers/builtin-registry.json" |
        ConvertFrom-Json
    $instructionRegistry = Get-Content -Raw `
        "xiaotao/references/instructions/builtin-registry.json" | ConvertFrom-Json
    $knownInstructionRefs = @{}
    foreach ($instruction in $instructionRegistry.instructions) {
        if ($knownInstructionRefs.ContainsKey($instruction.ref)) {
            throw "Built-in instruction registry contains duplicate ref '$($instruction.ref)'"
        }
        $knownInstructionRefs[$instruction.ref] = $true
        foreach ($sourcePath in $instruction.source_paths) {
            $instructionSource = Join-Path "xiaotao" $sourcePath
            if (-not (Test-Path -LiteralPath $instructionSource -PathType Leaf)) {
                throw "Instruction '$($instruction.ref)' references missing source '$sourcePath'"
            }
        }
    }
    $canonicalCapabilityPattern = '^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$'
    if (@($builtinRegistry.workers).Count -ne 0 -or
        @($builtinRegistry.aliases.PSObject.Properties).Count -ne 0) {
        throw "Built-in Worker registry must remain empty; new reusable Workers are project-owned"
    }

    $practiceRefs = @($instructionRegistry.instructions |
        Where-Object { $_.ref.StartsWith("practice:") } | ForEach-Object ref)
    if ($practiceRefs.Count -lt 1) {
        throw "Built-in instruction registry must expose capability practices"
    }
    $legacyRoleRefs = @($instructionRegistry.instructions |
        Where-Object { $_.ref.StartsWith("role:") })
    if ($legacyRoleRefs.Count -ne 0) {
        throw "Built-in instruction registry must not expose legacy role refs"
    }
    if (Test-Path -LiteralPath "xiaotao/references/roles") {
        throw "Legacy role reference directory must not be shipped"
    }

    $projectRegistry = Get-Content -Raw "$fixtureRoot/worker-registry-valid.json" | ConvertFrom-Json
    $projectCapabilities = @($projectRegistry.workers | ForEach-Object capabilities | Sort-Object -Unique)
    foreach ($worker in $projectRegistry.workers) {
        foreach ($capability in $worker.capabilities) {
            if ($capability -notmatch $canonicalCapabilityPattern) {
                throw "Project Worker '$($worker.id)' uses non-canonical capability '$capability'"
            }
        }
        foreach ($instructionRef in @($worker.instructions.required) + @($worker.instructions.optional)) {
            if (-not $knownInstructionRefs.ContainsKey($instructionRef)) {
                throw "Project Worker '$($worker.id)' references unknown instruction '$instructionRef'"
            }
        }
        if ($worker.instructions.required -notcontains "contract:handoff" -or
            $worker.instructions.required -notcontains "policy:safety-boundary" -or
            @($worker.instructions.required | Where-Object { $_.StartsWith("practice:") }).Count -lt 1) {
            throw "Project Worker '$($worker.id)' must declare a practice, Handoff, and safety contract"
        }
        if (@($worker.instructions.required | Where-Object { $_.StartsWith("role:") }).Count -gt 0) {
            throw "New project Worker '$($worker.id)' must not depend on a legacy role instruction"
        }
    }
    foreach ($alias in $projectRegistry.aliases.PSObject.Properties) {
        if ($alias.Name -notmatch $canonicalCapabilityPattern -or
            $alias.Value -notmatch $canonicalCapabilityPattern) {
            throw "Project registry contains a non-canonical capability alias"
        }
        if ($projectCapabilities -notcontains $alias.Value) {
            throw "Project capability alias '$($alias.Name)' targets unknown capability '$($alias.Value)'"
        }
    }

    function Test-ContainsEvery {
        param([object[]]$Available, [object[]]$Required)
        return @($Required | Where-Object { $Available -notcontains $_ }).Count -eq 0
    }

    $exactSelection = Get-Content -Raw "$fixtureRoot/worker-selection-exact-valid.json" |
        ConvertFrom-Json
    $exactCapabilities = @($exactSelection.requirements.required_capabilities) +
        @($exactSelection.requirements.optional_capabilities)
    $exactCandidates = @($projectRegistry.workers | Where-Object {
        Test-ContainsEvery $_.capabilities $exactCapabilities
    })
    if ($exactCandidates.Count -ne 1 -or
        $exactCandidates[0].id -ne $exactSelection.selected_workers[0].id) {
        throw "Exact selection fixture does not resolve uniquely from the project registry"
    }

    $composedSelection = Get-Content -Raw "$fixtureRoot/worker-selection-composed-valid.json" |
        ConvertFrom-Json
    $composedWorkers = @($composedSelection.selected_workers | ForEach-Object {
        $selectedId = $_.id
        $projectRegistry.workers | Where-Object id -eq $selectedId
    })
    if ($composedWorkers.Count -ne $composedSelection.selected_workers.Count) {
        throw "Composed selection contains an unknown project Worker"
    }
    $composedCapabilities = @($composedWorkers | ForEach-Object capabilities | Sort-Object -Unique)
    if (-not (Test-ContainsEvery $composedCapabilities `
        $composedSelection.requirements.required_capabilities)) {
        throw "Composed selection does not cover every required capability"
    }
    foreach ($worker in $composedWorkers) {
        if (Test-ContainsEvery $worker.capabilities $composedSelection.requirements.required_capabilities) {
            throw "Composed selection is not minimal because one Worker covers every requirement"
        }
    }

    $generatedSelection = Get-Content -Raw "$fixtureRoot/worker-selection-generated-valid.json" |
        ConvertFrom-Json
    $generatedWorker = Get-Content -Raw "$fixtureRoot/worker-temporary-valid.json" | ConvertFrom-Json
    if ($generatedWorker.id -ne $generatedSelection.selected_workers[0].id -or
        -not (Test-ContainsEvery $generatedWorker.capabilities `
            $generatedSelection.requirements.required_capabilities)) {
        throw "Generated selection and Task-scoped Worker fixture disagree"
    }
    if (-not (Test-ContainsEvery $generatedSelection.requirements.available_tools `
        $generatedWorker.tools)) {
        throw "Generated Worker requests a tool outside the requirements"
    }
    if (-not (Test-ContainsEvery $generatedSelection.requirements.context.read_paths `
        $generatedWorker.context.read_paths) -or
        -not (Test-ContainsEvery $generatedSelection.requirements.context.write_paths `
            $generatedWorker.context.write_paths)) {
        throw "Generated Worker context exceeds the requirements"
    }
    if (-not (Test-ContainsEvery $generatedSelection.requirements.permission_ceiling.autonomous `
        $generatedWorker.permissions.autonomous) -or
        -not (Test-ContainsEvery $generatedSelection.requirements.permission_ceiling.conditional `
            $generatedWorker.permissions.conditional)) {
        throw "Generated Worker permissions exceed the requirements ceiling"
    }
    if ($generatedWorker.lifecycle.scope -ne "task" -or
        $generatedWorker.lifecycle.expires_at -ne "task-completion") {
        throw "Generated Worker is not bounded to the Task lifecycle"
    }
    if (@($generatedWorker.instructions.required |
        Where-Object { $_.StartsWith("practice:") }).Count -lt 1 -or
        @($generatedWorker.instructions.required |
        Where-Object { $_.StartsWith("role:") }).Count -gt 0) {
        throw "Generated Worker must use capability practices instead of legacy role instructions"
    }
    $reusableGeneratedMatch = @($projectRegistry.workers | Where-Object {
        Test-ContainsEvery $_.capabilities $generatedSelection.requirements.required_capabilities
    })
    if ($reusableGeneratedMatch.Count -gt 0) {
        throw "Generated selection has a reusable match and should not generate a Worker"
    }

    $temporarySelection = Get-Content -Raw `
        "$fixtureRoot/worker-selection-temporary-valid.json" | ConvertFrom-Json
    $temporaryWorker = Get-Content -Raw `
        "$fixtureRoot/worker-temporary-memory-valid.json" | ConvertFrom-Json
    if ($temporaryWorker.id -ne $temporarySelection.selected_workers[0].id -or
        $temporaryWorker.lifecycle.scope -ne "temporary" -or
        $temporaryWorker.lifecycle.expires_at -ne "temporary-archive") {
        throw "Temporary selection and exploratory Worker lifecycle disagree"
    }
    if (-not (Test-ContainsEvery $temporarySelection.requirements.available_tools `
        $temporaryWorker.tools) -or
        -not (Test-ContainsEvery $temporarySelection.requirements.context.read_paths `
            $temporaryWorker.context.read_paths) -or
        -not (Test-ContainsEvery $temporarySelection.requirements.context.write_paths `
            $temporaryWorker.context.write_paths) -or
        -not (Test-ContainsEvery `
            $temporarySelection.requirements.permission_ceiling.autonomous `
            $temporaryWorker.permissions.autonomous) -or
        -not (Test-ContainsEvery `
            $temporarySelection.requirements.permission_ceiling.conditional `
            $temporaryWorker.permissions.conditional)) {
        throw "Temporary-scoped Worker exceeds its exploratory requirements"
    }
    if ($temporarySelection.selected_workers[0].snapshot_path -notmatch
        "/temporary/active/$([regex]::Escape($temporaryWorker.lifecycle.temporary_id))/") {
        throw "Temporary Worker snapshot is not stored under its lifecycle owner"
    }

    $sessionSelection = Get-Content -Raw `
        "$fixtureRoot/worker-selection-session-valid.json" | ConvertFrom-Json
    $sessionWorker = Get-Content -Raw "$fixtureRoot/worker-session-valid.json" | ConvertFrom-Json
    if ($sessionWorker.id -ne $sessionSelection.selected_workers[0].id -or
        $sessionWorker.lifecycle.scope -ne "session" -or
        $sessionWorker.lifecycle.expires_at -ne "session-end" -or
        $sessionSelection.selected_workers[0].ephemeral -ne $true) {
        throw "Session selection and ephemeral Worker lifecycle disagree"
    }
    if ($sessionWorker.tools.Count -gt 0 -or
        $sessionWorker.permissions.autonomous.Count -gt 0 -or
        $sessionWorker.permissions.conditional.Count -gt 0) {
        throw "Session Worker fixture exceeds its one-off requirements"
    }

    foreach ($selectionPath in @(
        "$fixtureRoot/worker-selection-exact-valid.json",
        "$fixtureRoot/worker-selection-composed-valid.json",
        "$fixtureRoot/worker-selection-generated-valid.json",
        "$fixtureRoot/worker-selection-temporary-valid.json",
        "$fixtureRoot/worker-selection-session-valid.json"
    )) {
        $selection = Get-Content -Raw $selectionPath | ConvertFrom-Json
        foreach ($selected in $selection.selected_workers) {
            if ($selected.ephemeral -eq $true) {
                if ($null -ne $selected.snapshot_path) {
                    throw "Ephemeral Worker '$($selected.id)' unexpectedly has a snapshot path"
                }
                continue
            }
            $expectedSuffix = "/workers/$($selected.id)/spec.yaml"
            if ($null -eq $selected.snapshot_path -or
                -not $selected.snapshot_path.EndsWith($expectedSuffix)) {
                throw "Selection snapshot path does not match Worker ID '$($selected.id)'"
            }
        }
    }

    $requiredContracts = @(
        @{ Path = "xiaotao/references/storage.md"; Text = "committed.yaml" },
        @{ Path = "xiaotao/references/storage.md"; Text = "before/<state-key>" },
        @{ Path = "xiaotao/references/storage.md"; Text = "staged/<state-key>" },
        @{ Path = "xiaotao/references/storage.md"; Text = "applied/<sequence>.yaml" },
        @{ Path = "xiaotao/references/storage.md"; Text = "SHA-256" },
        @{ Path = "xiaotao/references/storage.md"; Text = "checkpoint-observation.schema.json" },
        @{ Path = "xiaotao/references/activity.md"; Text = "checkpoint_recovered" },
        @{ Path = "xiaotao/references/activity.md"; Text = "worker_approved" },
        @{ Path = "xiaotao/references/workers.md"; Text = "worker-approval.schema.json" },
        @{ Path = "xiaotao/references/storage.md"; Text = "!.xiaotao/workers/approvals/" },
        @{ Path = "xiaotao/references/coordination.md"; Text = "Task 已是唯一逻辑活动目标" },
        @{ Path = "xiaotao/references/handoffs.md"; Text = '`needs_user_input: true` 要求 `status: blocked`' },
        @{ Path = "xiaotao/references/memory.md"; Text = "当前代码或运行时证据" },
        @{ Path = "xiaotao/references/workers.md"; Text = "Worker permissions 是请求的动作类别，不是授权" },
        @{ Path = "xiaotao/references/workers.md"; Text = "**能做什么**是当前委派中" },
        @{ Path = "xiaotao/references/workers.md"; Text = "直接修改 Long-term Memory" },
        @{ Path = "xiaotao/references/contract.md"; Text = '始终只有 `user_request` 是必需的' },
        @{ Path = "xiaotao/references/contract.md"; Text = "提案不能批准自身" },
        @{ Path = "xiaotao/references/memory.md"; Text = '`decision_context` 用在其他 memory kind 上无效' },
        @{ Path = "xiaotao/references/workers.md"; Text = "Task 或 Temporary 恢复时使用快照" },
        @{ Path = "xiaotao/references/workers.md"; Text = "Worker 不会隐式继承父 Agent 的完整 Skill" },
        @{ Path = "xiaotao/references/workers.md"; Text = "delegation-packet.schema.json" },
        @{ Path = "xiaotao/references/coordination.md"; Text = "不要假设 Worker 会继承" },
        @{ Path = "xiaotao/references/coordination.md"; Text = "Worker 解析不得把探索工作提升为 Task" },
        @{ Path = "xiaotao/references/coordination.md"; Text = "不得仅因协调器重新活跃就启动重复运行" },
        @{ Path = "xiaotao/SKILL.md"; Text = "通过宿主原生机制等待" },
        @{ Path = "xiaotao/references/workers.md"; Text = "scope: session" },
        @{ Path = "xiaotao/references/workers.md"; Text = "模型偏好绝不能自动提升它" },
        @{ Path = "xiaotao/references/coordination.md"; Text = "选择前，将有界委派转换为能力需求" },
        @{ Path = "xiaotao/references/coordination.md"; Text = "与 Task 或 Temporary 匹配" },
        @{ Path = "xiaotao/references/storage.md"; Text = "复制到匹配的 Task 或 Temporary" },
        @{ Path = "xiaotao/references/handoffs.md"; Text = ".xiaotao/memory/temporary/active/<temporary-id>/handoffs/" },
        @{ Path = "xiaotao/references/workers.md"; Text = '在所有宿主上使用 `/` 分隔符' },
        @{ Path = "xiaotao/references/handoffs.md"; Text = "由工件触发的协议守卫" },
        @{ Path = "xiaotao/references/memory.md"; Text = "此校验不得创建或转换 Task" },
        @{ Path = "xiaotao/references/memory.md"; Text = "UPDATE $([char]0x2192) MERGE $([char]0x2192) CREATE" },
        @{ Path = "xiaotao/references/memory.md"; Text = "这些动作是提案，不是写入" },
        @{ Path = "xiaotao/references/memory.md"; Text = '不能替代 `source_refs`' },
        @{ Path = "xiaotao/references/memory.md"; Text = "绝不要把 Temporary 或 Task 内容直接" },
        @{ Path = "xiaotao/references/memory.md"; Text = '`current_playbooks`' },
        @{ Path = "xiaotao/references/memory.md"; Text = '`request_file`' },
        @{ Path = "xiaotao/references/memory.md"; Text = '`--request`' },
        @{ Path = "xiaotao/references/memory.md"; Text = "match.playbook_ids $([char]0x2286) current_playbooks.playbook_id" },
        @{ Path = "xiaotao/references/memory.md"; Text = '`evidence_refs: []`' },
        @{ Path = "xiaotao/references/playbooks.md"; Text = "UPDATE $([char]0x2192) MERGE $([char]0x2192) CREATE $([char]0x2192) SKIP" },
        @{ Path = "xiaotao/references/playbooks.md"; Text = "用户明确批准" },
        @{ Path = "xiaotao/references/playbooks.md"; Text = "候选不是生效的指导" },
        @{ Path = "xiaotao/references/playbooks.md"; Text = "提供一次迁移" },
        @{ Path = "xiaotao/references/playbooks.md"; Text = '`revision: 0`' },
        @{ Path = "xiaotao/references/playbooks.md"; Text = "任意项目文件不能" },
        @{ Path = "xiaotao/references/storage.md"; Text = '包括 `SKIP`' },
        @{ Path = "xiaotao/references/storage.md"; Text = '`playbooks/candidates/`' },
        @{ Path = "xiaotao/references/storage.md"; Text = "规范正式 Playbook Markdown 或 YAML 文件" },
        @{ Path = "xiaotao/references/storage.md"; Text = '`superseded_by`' },
        @{ Path = "xiaotao/references/memory.md"; Text = "发现冲突 $([char]0x2192) pending-confirmation $([char]0x2192) resolved" },
        @{ Path = "xiaotao/references/memory.md"; Text = "防止已取代/拒绝 Memory 复活" },
        @{ Path = "xiaotao/references/storage.md"; Text = "团队共享 Memory（纳入 Git）" },
        @{ Path = "xiaotao/references/storage.md"; Text = "本地 Runtime 状态（不纳入 Git）" },
        @{ Path = "xiaotao/references/storage.md"; Text = "不读取、迁移或恢复旧 Role 目录" },
        @{ Path = "xiaotao/references/memory.md"; Text = '`long-term/entries/<entry_id>.md`' },
        @{ Path = "xiaotao/references/memory.md"; Text = "migrate-long-term" },
        @{ Path = "xiaotao/references/storage.md"; Text = "不同 entry 的独立 UPDATE 不共享 revision 或 lock" },
        @{ Path = "xiaotao/references/storage.md"; Text = '`completed_at` 是 Activity' },
        @{ Path = "xiaotao/references/activity.md"; Text = "不是新的权威状态源" },
        @{ Path = "xiaotao/references/activity.md"; Text = '不要直接读取完整 `activity/index.json`' },
        @{ Path = "xiaotao/references/activity.md"; Text = '没有 `record` 操作' },
        @{ Path = "xiaotao/references/activity.md"; Text = '`decision_approved` 和 `decision_superseded`' },
        @{ Path = "xiaotao/references/activity.md"; Text = '`playbook_approved` 和 `playbook_superseded`' },
        @{ Path = "xiaotao/references/playbooks.md"; Text = '### 不可变 Playbook 决策记录' },
        @{ Path = "xiaotao/references/storage.md"; Text = '`playbook_approved` / `playbook_superseded`' },
        @{ Path = "xiaotao/references/activity.md"; Text = '`temporary_promoted`' },
        @{ Path = "xiaotao/references/activity.md"; Text = '不产生晋升事件' },
        @{ Path = "xiaotao/references/activity.md"; Text = '`promotion_transaction` **不是**事件时间' },
        @{ Path = "xiaotao/references/storage.md"; Text = '`promotion_transaction` 是事务关联、恢复与审计标记，不是事件时间' },
        @{ Path = "xiaotao/references/storage.md"; Text = '必须在同一次生命周期更新中写入一次 `promoted_at`' },
        @{ Path = "xiaotao/references/storage.md"; Text = '`promoted_at` 是 Activity 中' },
        @{ Path = "xiaotao/references/memory.md"; Text = '`importance: milestone`' },
        @{ Path = "xiaotao/references/memory.md"; Text = '`decided_at` 是批准、取代或拒绝实际发生的时间' },
        @{ Path = "xiaotao/references/memory.md"; Text = '发布时必须严格验证其' },
        @{ Path = "xiaotao/references/memory.md"; Text = '后续读取只检查路径格式、项目内边界和不可逃逸' },
        @{ Path = "xiaotao/references/memory.md"; Text = 'validate.py decision-record <staged-record.json>' },
        @{ Path = "xiaotao/SKILL.md"; Text = "references/activity.md" },
        @{ Path = "README.md"; Text = "CLI 只负责安装、更新和诊断" },
        @{ Path = "README.md"; Text = "绝不调度 Worker" },
        @{ Path = "xiaotao/SKILL.md"; Text = "不负责编排工作" }
        @{ Path = "xiaotao/SKILL.md"; Text = "Worker、Memory、Playbook 或旧授权都不能扩权" }
        @{ Path = "xiaotao/SKILL.md"; Text = "唯一预置、直接面向用户的角色" }
    )
    foreach ($contract in $requiredContracts) {
        if (-not (Select-String -LiteralPath $contract.Path -SimpleMatch $contract.Text -Encoding utf8 -Quiet)) {
            throw "Missing contract '$($contract.Text)' in $($contract.Path)"
        }
    }

    $packageManifest = Get-Content -Raw "package.json" | ConvertFrom-Json
    if ($packageManifest.name -ne "xiaotao-ai-workflow" -or
        $packageManifest.bin.xiaotao -ne "bin/xiaotao.js") {
        throw "npm package metadata does not expose the expected XiaoTao CLI"
    }
    if ($null -ne $packageManifest.dependencies -and
        @($packageManifest.dependencies.PSObject.Properties).Count -gt 0) {
        throw "The multi-host installer CLI must remain runtime-dependency free"
    }

    $sceneRegistryContracts = @(
        "'.agents', 'skills', 'xiaotao'",
        "'.claude', 'skills', 'xiaotao'",
        "dsh-profile"
    )
    foreach ($sceneContract in $sceneRegistryContracts) {
        if (-not (Select-String -LiteralPath "cli/scenes.js" -SimpleMatch $sceneContract -Quiet)) {
            throw "Missing scene registry contract $sceneContract"
        }
    }

    $markdownFiles = Get-ChildItem "xiaotao" -Recurse -Filter "*.md"
    foreach ($document in $markdownFiles) {
        $content = Get-Content -Raw -Encoding utf8 $document.FullName
        $fenceCount = [regex]::Matches($content, '(?m)^```').Count
        if (($fenceCount % 2) -ne 0) {
            throw "Unbalanced code fences in $($document.FullName)"
        }

        $links = [regex]::Matches($content, "\[[^\]]+\]\(([^)#]+)(?:#[^)]+)?\)")
        foreach ($link in $links) {
            $relativeTarget = $link.Groups[1].Value
            if ($relativeTarget -match "^(https?://|mailto:)") {
                continue
            }
            $target = Join-Path $document.DirectoryName $relativeTarget
            if (-not (Test-Path -LiteralPath $target)) {
                throw "Broken local link in $($document.FullName): $relativeTarget"
            }
        }
    }

    Write-Output "All XiaoTao contract checks passed."
}
finally {
    Pop-Location
}

# Expected-invalid Ajv fixtures leave a native exit code of 1 even though the assertions passed.
# Return success explicitly; any thrown validation error exits before reaching this line.
exit 0
