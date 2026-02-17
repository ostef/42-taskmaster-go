#include <stdio.h>
#include <unistd.h>
#include <signal.h>

static int g_should_exit;

static void handle_signal(int sig) {
    if (sig == SIGUSR1) {
        printf("Yeah, yeah, I'm shutting down! Let me take a nap first though\n");
        sleep(10);
        g_should_exit = 1;
    } else {
        printf("Not my shutdown signal!\n");
    }
}

int main() {
    signal(SIGINT, handle_signal);
    signal(SIGTERM, handle_signal);
    signal(SIGUSR1, handle_signal);

    while (!g_should_exit) {
        sleep(1);
    }

    return 0;
}
