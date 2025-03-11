#include <iostream>
#include <thread>
#include <chrono>

int main() {
    std::this_thread::sleep_for(std::chrono::seconds(3));
    std::cout << "Hello, World!" << std::endl;
    return 0;
}
