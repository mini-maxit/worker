#include <iostream>
#include <thread>
#include <chrono>

void printSeconds() {
    for (int i = 1; i <= 5; ++i) {
        std::cout << "Second: " << i << std::endl;
        std::this_thread::sleep_for(std::chrono::seconds(1));
    }
}

int main() {
    printSeconds();
    return 0;
}
