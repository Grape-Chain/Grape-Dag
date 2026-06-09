package test

import (
	"fmt"
	"os"
	"path"
)

// func TestDagInit(t *testing.T) {
// 	fmt.Printf("Running %s\n", t.Name())
// 	x := config.GetGrapePeerFromConfig()
// 	// init random
// 	rand.Seed(time.Now().UnixMilli())
// 	d := dag.GenerateWideDag(uint8(x.Dag.Initialwidth))
// 	sites := d.GetTips()
// 	if len(sites) != int(x.Dag.Initialwidth) {
// 		t.Errorf("Incorrect initial width. Expected %d, got %d", x.Dag.Initialwidth, len(sites))
// 	}
// 	ok, count := d.Counters().IfCount(d.Genesis().GetID())
// 	if ok {
// 		if count != len(sites) {
// 			t.Errorf("Incorrect count %d. Expected %d", count, len(sites))
// 		}
// 	}
// 	fmt.Printf("%s - Ok\n", t.Name())
// }

func removeVis(name string) {
	wd, _ := os.Getwd()
	os.Remove(path.Join(wd, fmt.Sprintf("grapepeer.%s.*.gv", name)))
}

// func TestDagRandom(t *testing.T) {
// 	removeVis(t.Name())
// 	fmt.Printf("Running %s\n", t.Name())
// 	x := config.GetGrapePeerFromConfig()
// 	// init random
// 	rand.Seed(time.Now().UnixMilli())
// 	dagHeight := 4
// 	d := dag.GenerateRandomDag(uint8(x.Dag.Initialwidth), uint32(dagHeight))
// 	d.Visualize(t.Name())
// 	if uint16(d.Size()) != x.Dag.Initialwidth+uint16(dagHeight)+1 {
// 		t.Errorf("Incorrect Dag size. Expected %d, got %d", (x.Dag.Initialwidth + 101), d.Size())
// 	}
// 	sites := d.GetTips()
// 	if len(sites) < 4 {
// 		t.Errorf("Dag tips count is %d", len(sites))
// 	}
// 	confSites := dag.GetDag().GetConfirmedSites()
// 	if len(confSites) == 0 {
// 		t.Errorf("Confirmed sites count %d is invalid", len(confSites))
// 	}

// 	if uint16(d.Counters().SitesLen()) != x.Dag.Initialwidth+uint16(dagHeight)+1 {
// 		t.Errorf("Site Confirmation Counter value %d is incorrect", d.Counters().SitesLen())
// 	}
// 	fmt.Printf("%s - Ok\n", t.Name())
// }

// func TestConfirmedSites(t *testing.T) {
// 	removeVis(t.Name())
// 	fmt.Printf("Running %s\n", t.Name())
// 	x := config.GetGrapePeerFromConfig()
// 	// init random
// 	rand.Seed(time.Now().UnixMilli())
// 	var dagHeight uint16 = 1000
// 	d := dag.GenerateRandomDag(uint32(x.Dag.Initialwidth), uint32(dagHeight))
// 	d.Visualize(t.Name())
// 	if uint16(d.Size()) != x.Dag.Initialwidth+dagHeight+1 {
// 		t.Errorf("Incorrect Dag size. Expected %d, got %d\n", (x.Dag.Initialwidth + 101), d.Size())
// 	}
// 	confSites := d.GetConfirmedSites()
// 	if len(confSites) == 0 {
// 		t.Errorf("Confirmed sites count %d is invalid", len(confSites))
// 	}
// 	sites := d.GetTips()
// 	fmt.Printf("Num of confirmed tx - %d\n", len(sites))
// 	if uint16(d.Counters().SitesLen()) != x.Dag.Initialwidth+uint16(dagHeight)+1 {
// 		t.Errorf("Site Confirmation Counter value %d is incorrect", d.Counters().SitesLen())
// 	}
// 	fmt.Printf("%s - Ok\n", t.Name())
// }

// func TestConfirmedSitesN(t *testing.T) {
// 	fmt.Printf("Running %s\n", t.Name())
// 	x := config.GetGrapePeerFromConfig()
// 	// init random
// 	rand.Seed(time.Now().UnixMilli())
// 	count := []float64{}
// 	const iters int = 10
// 	x.Dag.Alpha = 0.1
// 	increment := 0.01
// 	for i := 0; i < iters; i++ {
// 		d := dag.GenerateRandomDag(uint32(x.Dag.Initialwidth), 100)
// 		tips := d.GetTips()
// 		var min float64 = 0.
// 		goterators.ForEach[*dag.Node](tips, func(n *dag.Node) {
// 			if float64(n.GetMaxHeight()) < min {
// 				min = float64(n.GetMaxHeight())
// 			}
// 		})

// 		if min < float64(iters)/2. {
// 			// orphaned tip - adjust alpha
// 			x.Dag.Alpha -= increment
// 			if x.Dag.Alpha <= 0 {
// 				x.Dag.Alpha = 0.01
// 			}
// 		} else {
// 			x.Dag.Alpha += increment
// 		}

// 		if uint16(d.Size()) != x.Dag.Initialwidth+100+1 {
// 			t.Errorf("Incorrect Dag size. Expected %d, got %d\n", (x.Dag.Initialwidth + 101), d.Size())
// 		}
// 		sites := d.GetConfirmedSites()
// 		count = append(count, float64(len(sites)))
// 		// if len(sites) < 40 {
// 		// 	t.Errorf("Dag tips count is %d", len(sites))
// 		// 	d.Visualize(fmt.Sprintf("Failed-%d-%d", len(sites), rand.Int31()))
// 		// }
// 	}
// 	smean, _ := stats.Mean(count)
// 	fmt.Printf("Mean	: %f\n", smean)
// 	sstd, _ := stats.StandardDeviation(count)
// 	fmt.Printf("Std Div	: %f\n", sstd)
// 	smin, _ := stats.Min(count)
// 	fmt.Printf("Min		: %f\n", smin)
// 	smax, _ := stats.Max(count)
// 	fmt.Printf("Max		: %f\n", smax)
// 	fmt.Printf("Alpha : %f\n", x.Dag.Alpha)
// 	if smean < float64(iters)*2.0/3.0 {
// 		t.Errorf("Failed with smean less than 2/3 of total iterations. Please tune RW algorithm\n")
// 	}

// 	fmt.Printf("%s - Ok\n", t.Name())
// }
