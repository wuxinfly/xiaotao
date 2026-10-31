# Directory Layout Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Separate the Skill toolkit from the future client.

**Architecture:** Move the existing npm package into xt-skills, reserve xt-client, and keep website, docs and repository metadata at root.

**Tech Stack:** Node.js ESM, Python, PowerShell, GitHub Actions.

---

### Task 1: Move toolkit
Move bin, cli, adapters, xiaotao, scripts, test, manifests, npm ignore and package README into xt-skills. Preserve internal relative paths.

### Task 2: Update entry points
Update CI working directories, website and documentation links, root ignore and attributes, repository memory fingerprints and its integration test. Add root and Skill AGENTS.md and client placeholder.

### Task 3: Verify
Run main package tests, contracts and pack dry run; adapter typecheck, tests and package checks; website build. Check whitespace and ensure relocated tracked files remain visible to Git. Do not publish, install into hosts or deploy.
