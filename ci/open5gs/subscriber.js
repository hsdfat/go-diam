// One synthetic subscriber for the Open5GS HSS, run with mongosh against the
// "open5gs" database. The document layout is the one Open5GS's own
// provisioning tool writes. IMSI in the test PLMN 001/01; K and OPc are
// random test values generated for this file and used nowhere else.
db.subscribers.deleteMany({});
db.subscribers.insertOne({
  schema_version: NumberInt(1),
  imsi: "001010000000001",
  msisdn: ["0000000001"],
  imeisv: [],
  mme_host: [],
  mm_realm: [],
  purge_flag: [],
  slice: [{
    sst: NumberInt(1),
    default_indicator: true,
    session: [{
      name: "internet",
      type: NumberInt(3),
      qos: {
        index: NumberInt(9),
        arp: {
          priority_level: NumberInt(8),
          pre_emption_capability: NumberInt(1),
          pre_emption_vulnerability: NumberInt(2),
        },
      },
      ambr: {
        downlink: { value: NumberInt(1000000000), unit: NumberInt(0) },
        uplink: { value: NumberInt(1000000000), unit: NumberInt(0) },
      },
      pcc_rule: [],
    }],
  }],
  security: {
    k: "8fb3b835bcfcf55fd9c6413aa4819526",
    op: null,
    opc: "5557cf61c0e6d150f973cf5bed058819",
    amf: "8000",
    sqn: NumberLong(64),
  },
  ambr: {
    downlink: { value: NumberInt(1000000000), unit: NumberInt(0) },
    uplink: { value: NumberInt(1000000000), unit: NumberInt(0) },
  },
  access_restriction_data: 32,
  network_access_mode: 0,
  subscriber_status: 0,
  operator_determined_barring: 0,
  subscribed_rau_tau_timer: 12,
  __v: 0,
});
printjson(db.subscribers.find({}, { _id: 0, imsi: 1, msisdn: 1 }).toArray());
