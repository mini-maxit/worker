#!/bin/bash

# Set up the root directory for chroot
CHROOT_DIR="/tmp/chroot"

# Create necessary directories
echo "Creating directory structure in ${CHROOT_DIR}..."
mkdir -p "$CHROOT_DIR"/{bin,lib/x86_64-linux-gnu,usr/lib/x86_64-linux-gnu,usr/bin,usr/sbin,etc/security}

# List of esential binaries
ESSENTIAL_BINARIES="/bin/bash /bin/apt /usr/bin/apt-get /usr/bin/apt-cache /usr/sbin/apt /usr/bin/timeout"

# Copy essential binaries for apt, prlimit, and utilities into chroot
echo "Copying apt, prlimit, and utilities to chroot..."
for binary in $ESSENTIAL_BINARIES; do
  if [[ -f "$binary" ]]; then
    dest_dir="$CHROOT_DIR$(dirname "$binary")"
    mkdir -p "$dest_dir"
    cp "$binary" "$dest_dir"
  else
    echo "Warning: $binary not found, skipping."
  fi
done

# Copy necessary libraries for apt tools, prlimit, and timeout
copyLibs() {
  local binary="$1"
  ldd "$binary" | grep -o '/[^ ]*' | while read -r lib; do
    if [[ -f "$lib" ]]; then
      lib_dir="$CHROOT_DIR$(dirname "$lib")"
      mkdir -p "$lib_dir"
      cp -n "$lib" "$lib_dir"
    fi
  done
}

# Copy libraries for all apt-related binaries, prlimit, and timeout
for binary in $ESSENTIAL_BINARIES; do
  copyLibs "$binary"
done

# Copy necessary shell binaries and libraries
echo "Copying shell and its dependencies..."
cp /bin/bash "$CHROOT_DIR/bin/bash"
copyLibs /bin/bash

# Copy C++ standard libraries and dependencies
echo "Copying C++ standard libraries and dependencies..."
copyLibs "/usr/lib/x86_64-linux-gnu/libstdc++.so.6"
copyLibs "/lib/x86_64-linux-gnu/libgcc_s.so.1"


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

# Install g++ inside chroot
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
