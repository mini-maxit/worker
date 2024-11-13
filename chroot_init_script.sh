#!/bin/bash

# Set up the root directory for chroot
CHROOT_DIR="/tmp/chroot"

# Create necessary directories
echo "Creating directory structure in ${CHROOT_DIR}..."
mkdir -p "$CHROOT_DIR"/{bin,lib/x86_64-linux-gnu,usr/lib/x86_64-linux-gnu,usr/bin,usr/sbin}

# Copy essential binaries for apt and utilities into chroot
echo "Copying apt and utilities to chroot..."
for binary in /bin/bash /bin/apt /usr/bin/apt-get /usr/bin/apt-cache /usr/sbin/apt; do
  if [[ -f "$binary" ]]; then
    dest_dir="$CHROOT_DIR$(dirname "$binary")"
    mkdir -p "$dest_dir"
    cp "$binary" "$dest_dir"
  else
    echo "Warning: $binary not found, skipping."
  fi
done

# Copy necessary libraries for apt tools
copy_libs() {
  local binary="$1"
  # Use `ldd` to get the list of dependencies and copy them
  ldd "$binary" | grep -o '/[^ ]*' | while read -r lib; do
    if [[ -f "$lib" ]]; then
      # Get directory path within chroot
      lib_dir="$CHROOT_DIR$(dirname "$lib")"
      mkdir -p "$lib_dir"
      cp -n "$lib" "$lib_dir"
    fi
  done
}

# Copy libraries for all apt-related binaries
for binary in /bin/apt /usr/bin/apt-get /usr/bin/apt-cache /usr/sbin/apt; do
  copy_libs "$binary"
done

# Copy the apt configuration and sources.list to chroot
echo "Copying apt configuration files..."
mkdir -p "$CHROOT_DIR/etc/apt"
cp -r /etc/apt/sources.list /etc/apt/apt.conf.d /etc/apt/preferences.d "$CHROOT_DIR/etc/apt/"

# Ensure necessary directories are in place for dpkg
echo "Setting up dpkg directories in chroot..."
mkdir -p "$CHROOT_DIR/var/lib/dpkg"
mkdir -p "$CHROOT_DIR/var/cache/apt/archives"

# Mount proc and sys to allow package installation
echo "Mounting proc and sys for chroot environment..."
sudo mount -t proc proc "$CHROOT_DIR/proc"
sudo mount -t sysfs sys "$CHROOT_DIR/sys"

# Chroot into the environment and update package list, then install g++
echo "Chrooting into the environment and installing g++..."
sudo chroot "$CHROOT_DIR" /bin/bash -c "
  apt-get update &&
  apt-get install -y g++ &&
  apt-get clean
"

# Unmount proc and sys from chroot
echo "Unmounting proc and sys from chroot..."
sudo umount "$CHROOT_DIR/proc"
sudo umount "$CHROOT_DIR/sys"

echo "g++ and dependencies have been installed inside the chroot."
