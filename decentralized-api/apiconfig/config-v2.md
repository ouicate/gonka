## Dynamic data

1. Nodes                                   []InferenceNodeConfig
2. NodeConfigIsMerged                      bool # Will be deleted
3. UpcomingSeed/CurrentSeed/PreviousSeed   SeedInfo
4. CurrentHeight                           int64/LastProcessedHeight int64 
5. UpgradePlan                             UpgradePlan
6. MLNodeKeyConfig                         MLNodeKeyConfig???
7. CurrentNodeVersion/LastUsedVersion      string # Related to UpgradePlan??
8. ValidationParams                        ValidationParamsCache
9. BandwidthParams                         BandwidthParamsCache
