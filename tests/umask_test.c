#include <stdio.h>
#include <error.h>
#include <errno.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <sys/types.h>
#include <sys/stat.h>

int main() {
    FILE *file = fopen("test-umask.txt", "wb");
    if (!file) {
        printf("Error: %s\n", strerror(errno));
    }
}
