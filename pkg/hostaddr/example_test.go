package hostaddr_test

import (
	"fmt"

	"github.com/shigabutdinoff/metrics/pkg/hostaddr"
)

func ExampleHost() {
	fmt.Println(hostaddr.Host("10.0.0.5:34567"))
	fmt.Println(hostaddr.Host("10.0.0.5"))

	// Output:
	// 10.0.0.5
	// 10.0.0.5
}
