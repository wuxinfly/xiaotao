# Active Temporary Memory Routing Design

## Problem

XiaoTao may keep several Temporary Memories active at the same time, but it currently has no
contract for deciding which one a later request should resume. Choosing the newest directory or
loading every historical Reference can silently contaminate an unrelated topic.

## Design

Routing uses a strict precedence order while leaving natural-language interpretation flexible:

1. An explicit Temporary ID, unique topic, alias, or user selection wins.
2. A valid binding in the current Session continues unless the user clearly switches topics.
3. Without a binding, XiaoTao may auto-select only one uniquely relevant candidate supported by
   specific routing evidence from lightweight metadata and current state.
4. If a second candidate remains plausibly relevant, XiaoTao asks the user to choose.
5. If no candidate is meaningfully related, XiaoTao treats the request as a new topic and creates
   Temporary Memory only when the existing persistence rules call for it.

The policy deliberately avoids numeric semantic scores. Models and hosts may understand language
differently, while the permission to auto-route must remain stable: a unique, explainable match is
required, and ambiguity always produces a short confirmation question.

## Context and metadata

Routing initially reads only `meta.yaml` and the routing sections of `current.md`. Historical
References are excluded until after selection. Required metadata identifies and describes the
Temporary; aliases and a stable host Session ID are optional. A host without stable Session IDs may
keep the binding only in current conversational context.

Switching a binding never merges, archives, or modifies the previous Temporary. Recency may order
candidate presentation but cannot turn an ambiguous semantic result into an automatic selection.

## Verification

Behavior scenarios cover zero, one, and multiple active candidates; explicit overrides; Session
continuity; ambiguous requests; unrelated requests; invalid explicit IDs; and lightweight loading.
