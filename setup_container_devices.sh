#!/bin/bash
ROOTFS="$1"

# Create device directory
mkdir -p "$ROOTFS/dev"

# Essential device files
mknod "$ROOTFS/dev/null" c 1 3 2>/dev/null || true
mknod "$ROOTFS/dev/zero" c 1 5 2>/dev/null || true
mknod "$ROOTFS/dev/random" c 1 8 2>/dev/null || true  
mknod "$ROOTFS/dev/urandom" c 1 9 2>/dev/null || true

# Set permissions
chmod 666 "$ROOTFS/dev/null" "$ROOTFS/dev/zero" "$ROOTFS/dev/random" "$ROOTFS/dev/urandom"

# Create /dev/pts for terminal support
mkdir -p "$ROOTFS/dev/pts" 2>/dev/null || true
