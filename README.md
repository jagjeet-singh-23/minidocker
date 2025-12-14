# MiniDocker - A Container Runtime Built from Scratch

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Linux](https://img.shields.io/badge/Linux-Required-FCC624?style=flat&logo=linux&logoColor=black)](https://www.kernel.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A Docker-like container runtime built from scratch in Go to understand containerization fundamentals.

## 🎯 Project Goal

Understanding how containers really work by implementing the core primitives: Linux namespaces, cgroups, OverlayFS, and networking from the ground up.

## ✨ Features

- **Process Isolation**: Linux namespaces (PID, Mount, UTS, Network)
- **Resource Management**: cgroups v2 (memory & CPU limits)
- **Networking**: Bridge networking with NAT and port forwarding
- **Storage**: OverlayFS-based layered images with copy-on-write
- **Volumes**: Named volumes and bind mounts
- **Lifecycle**: Full container lifecycle (run, stop, exec, logs, commit)
- **Images**: Multi-layer images with SHA256 content addressing

## 📚 What I Learned

This project taught me:
- How containers are just isolated Linux processes
- Why OverlayFS enables efficient image storage
- How Docker networking really works (veth pairs, bridges, NAT)
- The beauty of Linux kernel features working together
