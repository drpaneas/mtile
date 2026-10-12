package transpiler

func canonicalPoseCount(opts Options) int {
	if opts.PoseVariantBaseCount > 0 {
		return opts.PoseVariantBaseCount
	}
	return len(opts.PoseIndex)
}
func expandedPoseIDs(opts Options, ids []int) []int {
	if opts.PoseVariantBaseCount == 0 {
		return ids
	}
	expanded := make([]int, 0, len(ids)*(len(opts.PoseVariantNames)+1))
	for _, id := range ids {
		if id < 0 || id >= opts.PoseVariantBaseCount {
			continue
		}
		for v := 0; v <= len(opts.PoseVariantNames); v++ {
			expanded = append(expanded, v*opts.PoseVariantBaseCount+id)
		}
	}
	return expanded
}
