package service

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/fnikolai/frisbee/api/v1alpha1"
	"github.com/fnikolai/frisbee/controllers/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsMacro(macro string) bool {
	return strings.HasPrefix(macro, ".")
}

func parseMacro(ss *v1alpha1.ServiceSelector) {
	fields := strings.Split(*ss.Macro, ".")

	if len(fields) != 4 {
		panic(errors.Errorf("%s is not a valid macro", *ss.Macro))
	}

	kind := fields[1]
	object := fields[2]
	filter := fields[3]

	switch kind {
	case "servicegroup":
		ss.Match.ServiceGroup = object
		ss.Mode = v1alpha1.Mode(filter)

	default:
		panic(errors.Errorf("%v is not a valid macro", ss.Macro))
	}
}

func Select(ctx context.Context, ss *v1alpha1.ServiceSelector) common.ServiceList {
	if ss == nil {
		logrus.Warn("empty service selector")

		return []v1alpha1.Service{}
	}

	if ss.Macro != nil {
		parseMacro(ss)
	}

	// get all available services that match the criteria
	services, err := selectServices(ctx, &ss.Match)
	if err != nil {
		logrus.Warn(err)

		return []v1alpha1.Service{}
	}

	if len(services) == 0 {
		return []v1alpha1.Service{}
	}

	// filter services based on the pods
	filteredServices, err := filterServicesByMode(services, ss.Mode, ss.Value)
	if err != nil {
		logrus.Warn(err)

		return []v1alpha1.Service{}
	}

	return filteredServices
}

func selectServices(ctx context.Context, ss *v1alpha1.MatchServiceSpec) ([]v1alpha1.Service, error) {
	if ss == nil {
		return nil, nil
	}

	var services []v1alpha1.Service

	var listOptions client.ListOptions

	// case 1. services are specifically specified
	if len(ss.ServiceNames) > 0 {
		for ns, names := range ss.ServiceNames {
			for _, name := range names {
				var service v1alpha1.Service

				key := client.ObjectKey{
					Namespace: ns,
					Name:      name,
				}

				if err := common.Common.Client.Get(ctx, key, &service); err != nil {
					return nil, errors.Wrapf(err, "unable to find %s", key)
				}

				services = append(services, service)
			}
		}
	}

	// case 2. servicegroups are specifically specified. Owner labels ia automatically attached to all
	// components by SetOwnerRef(). Thus, we are looking for services that are owned by the desired servicegroup.
	if len(ss.ServiceGroup) > 0 {
		ss.Labels = labels.Merge(ss.Labels, map[string]string{
			"owner": ss.ServiceGroup,
		})
	}

	// case 3. labels
	if len(ss.Labels) > 0 {
		ls, err := metav1.LabelSelectorAsSelector(metav1.SetAsLabelSelector(ss.Labels))
		if err != nil {
			return nil, err
		}

		listOptions = client.ListOptions{LabelSelector: ls}
	}

	var serviceList v1alpha1.ServiceList

	// case 4. ByNamespace
	if len(ss.Namespaces) > 0 { // search specified namespaces
		for _, namespace := range ss.Namespaces {
			listOptions.Namespace = namespace

			if err := common.Common.Client.List(ctx, &serviceList, &listOptions); err != nil {
				return nil, err
			}

			services = append(services, serviceList.Items...)
		}
	} else { // search all namespaces
		if err := common.Common.Client.List(ctx, &serviceList, &listOptions); err != nil {
			return nil, errors.Wrapf(err, "namespace error")
		}

		services = append(services, serviceList.Items...)
	}

	// select services For more options see
	// https://github.com/chaos-mesh/chaos-mesh/blob/31aef289b81a1d713b5a9976a257090da81ac29e/pkg/selector/pod/selector.go

	return services, nil
}

func filterServicesByMode(services []v1alpha1.Service, mode v1alpha1.Mode, value string) ([]v1alpha1.Service, error) {
	if len(services) == 0 {
		return nil, errors.New("cannot generate services from empty list")
	}

	switch mode {
	case v1alpha1.OneMode:
		index := getRandomNumber(len(services))
		service := services[index]

		return []v1alpha1.Service{service}, nil
	case v1alpha1.AllMode:
		return services, nil

	case v1alpha1.FixedMode:
		num, err := strconv.Atoi(value)
		if err != nil {
			return nil, err
		}

		if len(services) < num {
			num = len(services)
		}

		if num <= 0 {
			return nil, errors.New("cannot select any service as value below or equal 0")
		}

		return getFixedSubListFromServiceList(services, num), nil
	case v1alpha1.FixedPercentMode:
		percentage, err := strconv.Atoi(value)
		if err != nil {
			return nil, err
		}

		if percentage == 0 {
			return nil, errors.New("cannot select any pod as value below or equal 0")
		}

		if percentage < 0 || percentage > 100 {
			return nil, errors.Errorf("fixed percentage value of %d is invalid, Must be (0,100]", percentage)
		}

		num := int(math.Floor(float64(len(services)) * float64(percentage) / 100))

		return getFixedSubListFromServiceList(services, num), nil
	case v1alpha1.RandomMaxPercentMode:
		maxPercentage, err := strconv.Atoi(value)
		if err != nil {
			return nil, err
		}

		if maxPercentage == 0 {
			return nil, errors.New("cannot select any pod as value below or equal 0")
		}

		if maxPercentage < 0 || maxPercentage > 100 {
			return nil, errors.Errorf("fixed percentage value of %d is invalid, Must be [0-100]", maxPercentage)
		}

		// + 1 because Intn works with half open interval [0,n) and we want [0,n]
		percentage := getRandomNumber(maxPercentage + 1)
		num := int(math.Floor(float64(len(services)) * float64(percentage) / 100))

		return getFixedSubListFromServiceList(services, num), nil
	default:
		return nil, errors.Errorf("mode %s not supported", mode)
	}
}

func getRandomNumber(max int) uint64 {
	num, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return num.Uint64()
}

func getFixedSubListFromServiceList(services []v1alpha1.Service, num int) []v1alpha1.Service {
	indexes := RandomFixedIndexes(0, uint(len(services)), uint(num))

	filteredServices := make([]v1alpha1.Service, len(indexes))

	for _, index := range indexes {
		index := index
		filteredServices = append(filteredServices, services[index])
	}

	return filteredServices
}

// RandomFixedIndexes returns the `count` random indexes between `start` and `end`.
// [start, end)
func RandomFixedIndexes(start, end, count uint) []uint {
	var indexes []uint

	m := make(map[uint]uint, count)

	if end < start {
		return indexes
	}

	if count > end-start {
		for i := start; i < end; i++ {
			indexes = append(indexes, i)
		}

		return indexes
	}

	for i := 0; i < int(count); {
		index := uint(getRandomNumber(int(end-start))) + start

		_, exist := m[index]
		if exist {
			continue
		}

		m[index] = index
		indexes = append(indexes, index)
		i++
	}

	return indexes
}
