#!/bin/bash

# Set up the root directory for chroot
CHROOT_DIR="/tmp/chroot"

# Create necessary directories
echo "Creating directory structure in ${CHROOT_DIR}..."
mkdir -p "$CHROOT_DIR"/{bin,lib/x86_64-linux-gnu,usr/lib/x86_64-linux-gnu,usr/bin,usr/sbin,etc/security,usr/lib}

# List of essential binaries
ESSENTIAL_BINARIES="/bin/bash /bin/apt /usr/bin/apt-get /usr/bin/apt-cache /usr/sbin/apt /usr/bin/timeout /bin/ls /usr/bin/python3 /usr/bin/env"

# Copy essential binaries for apt, prlimit, utilities, ls, python3, and env into chroot
echo "Copying apt, prlimit, utilities, ls, python3, and env to chroot..."
for binary in $ESSENTIAL_BINARIES; do
  if [[ -f "$binary" ]]; then
    dest_dir="$CHROOT_DIR$(dirname "$binary")"
    mkdir -p "$dest_dir"
    cp "$binary" "$dest_dir"
  else
    echo "Warning: $binary not found, skipping."
  fi
done

# Copy necessary libraries for apt tools, prlimit, timeout, ls, python3, and env
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

# Copy libraries for all apt-related binaries, prlimit, timeout, ls, python3, and env
for binary in $ESSENTIAL_BINARIES; do
  copyLibs "$binary"
done

# Ensure Python dependencies are copied
echo "Copying Python shared libraries..."
PYTHON_LIBS=$(ldd /usr/bin/python3 | awk '{print $3}' | grep '^/')
for lib in $PYTHON_LIBS; do
  copyLibs "$lib"
done

# Copy the entire Python standard library
echo "Copying Python standard library..."
mkdir -p "$CHROOT_DIR/usr/lib"
cp -r /usr/lib/python3.* "$CHROOT_DIR/usr/lib/"

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

# Install g++ and python3 inside chroot
echo "Chrooting into the environment and installing g++ and python3..."
sudo chroot "$CHROOT_DIR" /bin/bash -c "
  apt-get update &&
  apt-get install -y g++ python3 &&
  apt-get clean
"

# Unmount proc and sys from chroot
echo "Unmounting proc and sys from chroot..."
sudo umount "$CHROOT_DIR/proc"
sudo umount "$CHROOT_DIR/sys"

echo "g++, python3, and dependencies have been installed inside the chroot."

echo "Setting up Python environment inside chroot..."
sudo chroot "$CHROOT_DIR" /bin/bash -c "
  export PYTHONHOME='/usr' &&
  export PYTHONPATH='/usr/lib/python3.11' &&
  python3 --version
"

echo "Python should now work properly inside the chroot."
