#!/usr/bin/env python3
"""rdf-sidecar: OWL/RDF ↔ spec_json 双向转换（方案 04 v0.6 §2.2/§6，不自研 RDF 解析）。

用法:
  sidecar.py parse  --format owl_rdfxml|turtle|jsonld < input   -> stdout JSON {spec, warnings, lossy, note}
  sidecar.py export --ontology-id ID [--format turtle] < spec_json -> stdout TTL

导入映射（REQ-69 细则定稿，REQ-235/M62 扩 SKOS/JSON-LD/数据属性档）:
  rdfs:label|skos:prefLabel（pref/alt 优先）→ name/label、rdfs:comment|skos:definition → definition、
  rdfs:subClassOf|skos:broader|skos:narrower(反向) → parents、
  owl:Class|rdfs:Class|skos:Concept → concept、
  owl:NamedIndividual+rdf:type → instance、对象属性断言 → 实例关系、
  datatype 属性断言 → attributes（实例级）；owl:DatatypeProperty 声明按**固定策略一档**降级：
  不入 relations、域约束与属性公理丢弃、逐属性 warning 明示（REQ-235② 拍板：不做导入偏好配置）；
  其余公理/推理语义丢弃并计入 warnings。
导出 URN 规则与 pkg/ontology/spec 的 sanitize 逐字符一致（查询翻译共用）。
"""
import sys, json

try:
    from rdflib import Graph, RDF, RDFS, OWL, URIRef, Literal, SKOS
except ImportError:
    sys.stderr.write("rdflib 未安装：pip install rdflib\n")
    sys.exit(3)

RDFXML = "xml"
TTL = "turtle"
JSONLD = "json-ld"


def sanitize(s: str) -> str:
    return "".join(ch if (ch.isalnum() or ch in "_-.") else "_" for ch in (s or "").strip())


def local(uri: str) -> str:
    for sep in ("#", "/", ":"):
        if sep in uri:
            uri = uri.rsplit(sep, 1)[-1]
    return uri


def label_or_local(g, subj) -> str:
    # REQ-235①：SKOS prefLabel/altLabel 优先于 rdfs:label（SKOS 词表常无 rdfs:label）
    for pred in (SKOS.prefLabel, SKOS.altLabel, RDFS.label):
        for lab in g.objects(subj, pred):
            s = str(lab).strip()
            if s:
                return s
    return local(str(subj))


def definition_of(g, subj) -> str | None:
    # REQ-235①：skos:definition 与 rdfs:comment 等价承载定义
    for pred in (RDFS.comment, SKOS.definition):
        v = g.value(subj, pred)
        if v and str(v).strip():
            return str(v)
    return None


def parse(format: str) -> int:
    content = sys.stdin.read()
    g = Graph()
    fmt = {"owl_rdfxml": RDFXML, "turtle": TTL, "jsonld": JSONLD}.get(format)
    if fmt is None:
        sys.stderr.write(f"未知解析格式 {format}\n")
        return 2
    try:
        g.parse(data=content, format=fmt)
    except Exception as e:  # noqa: BLE001
        sys.stderr.write(f"RDF 解析失败: {e}\n")
        return 1

    warnings, lossy = [], False
    spec = {"name": "", "description": "", "concepts": [], "relations": [], "instances": []}

    # ---- 类：owl:Class / rdfs:Class / skos:Concept（REQ-235① SKOS 词表；跳过系统内建类）----
    seen_c = {}
    classes = set(g.subjects(RDF.type, OWL.Class)) | set(g.subjects(RDF.type, RDFS.Class)) | set(g.subjects(RDF.type, SKOS.Concept))
    for c in classes:
        if not isinstance(c, URIRef):
            continue
        uri = str(c)
        if uri.startswith(str(RDFS)) or uri.startswith(str(OWL)) or uri.startswith(str(RDF)) or uri.startswith(str(SKOS)):
            continue
        name = label_or_local(g, c)
        if name in seen_c:
            warnings.append(f"重复概念名 {name}（{uri}）已跳过")
            continue
        seen_c[uri] = name
        concept = {"name": name}
        comment = definition_of(g, c)
        if comment:
            concept["definition"] = comment
        parents = []
        for p in g.objects(c, RDFS.subClassOf):
            if isinstance(p, URIRef) and str(p) in seen_c:
                parents.append(seen_c[str(p)])
            elif isinstance(p, URIRef) and str(p) not in seen_c:
                # 父类尚未注册（外部类）——先记占位，后续第二轮补
                pass
            else:
                lossy = True
                warnings.append(f"{name} 的匿名父类（限制/公理）丢弃: {p.n3()[:80]}")
        # REQ-235①：skos:broader/narrower → parents 统一放第二轮（父注册完成后补，避免悬空引用）
        if parents:
            concept["parents"] = parents
        spec["concepts"].append(concept)

    # 第二轮补父类（前向引用；含 SKOS broader/narrower——REQ-235①）
    by_name = {c["name"]: c for c in spec["concepts"]}
    uri_by_name = {v: k for k, v in seen_c.items()}
    name_by_uri = {v: k for k, v in uri_by_name.items()}
    for c in classes:
        if not isinstance(c, URIRef) or str(c) not in seen_c:
            continue
        mine = by_name.get(seen_c[str(c)])
        if not mine:
            continue
        parents = []
        for p in g.objects(c, RDFS.subClassOf):
            if isinstance(p, URIRef):
                pname = name_by_uri.get(str(p))
                if pname and pname != mine["name"]:
                    parents.append(pname)
        # skos:broader → parents；skos:narrower 反向 → parents（目标须已注册为概念，外部引用 warning 丢弃）
        for p in list(g.objects(c, SKOS.broader)) + list(g.subjects(SKOS.narrower, c)):
            if not isinstance(p, URIRef):
                continue
            pname = name_by_uri.get(str(p))
            if pname and pname != mine["name"] and pname not in parents:
                parents.append(pname)
            elif pname is None and not parents:
                lossy = True
                warnings.append(f"{mine['name']} 的 skos:broader/narrower 目标 {local(str(p))} 未注册为概念，丢弃")
        if parents:
            mine["parents"] = parents

    # ---- 对象属性 ----
    seen_r = {}
    props = set(g.subjects(RDF.type, OWL.ObjectProperty))
    for pr in props:
        if not isinstance(pr, URIRef):
            continue
        uri = str(pr)
        if uri.startswith(str(RDF)) or uri.startswith(str(RDFS)) or uri.startswith(str(OWL)):
            continue
        name = label_or_local(g, pr)
        if name in seen_r:
            warnings.append(f"重复关系名 {name} 已跳过")
            continue
        seen_r[uri] = name
        rel = {"name": name}
        comment = g.value(pr, RDFS.comment)
        if comment:
            rel["definition"] = str(comment)
        dom = g.value(pr, RDFS.domain)
        rng = g.value(pr, RDFS.range)
        if dom is not None and str(dom) in seen_c:
            rel["from"] = seen_c[str(dom)]
        else:
            rel["from"] = ""
            lossy = True
            warnings.append(f"关系 {name} 定义域缺失或未注册为概念")
        if rng is not None and str(rng) in seen_c:
            rel["to"] = seen_c[str(rng)]
        else:
            rel["to"] = ""
            lossy = True
            warnings.append(f"关系 {name} 值域缺失或未注册为概念")
        spec["relations"].append(rel)
    # REQ-235②：datatype 属性声明按**固定策略一档**降级（拍板：不做导入偏好配置）——
    # 声明不入 relations；实例断言落 attributes（实例段统一处理）；域约束与属性公理丢弃，逐属性 warning 明示。
    dt_props = set(g.subjects(RDF.type, OWL.DatatypeProperty))
    for pr in dt_props:
        if not isinstance(pr, URIRef) or str(pr).startswith((str(RDF), str(RDFS), str(OWL), str(SKOS))):
            continue
        lossy = True
        dom = g.value(pr, RDFS.domain)
        dom_s = f"（domain {label_or_local(g, dom)} 丢弃）" if dom is not None else ""
        warnings.append(f"datatype 属性 {label_or_local(g, pr)} 按固定策略降级为实例 attributes{dom_s}（属性公理丢弃）")

    # ---- 实例 ----
    inst_by_uri = {}
    for ind in g.subjects(RDF.type, OWL.NamedIndividual):
        if not isinstance(ind, URIRef):
            continue
        uri = str(ind)
        name = label_or_local(g, ind)
        if name in inst_by_uri:
            warnings.append(f"重复实例名 {name} 已跳过")
            continue
        inst_by_uri[uri] = name
        it = {"name": name, "attributes": {}, "relations": []}
        ctypes = [str(t) for t in g.objects(ind, RDF.type)
                  if isinstance(t, URIRef) and str(t) in seen_c and str(t) != str(OWL.NamedIndividual)]
        if ctypes:
            it["concept"] = seen_c[ctypes[0]]
        else:
            # REQ-235：类型未注册为概念的实例丢弃（置空保留会产生过不了结构校验的悬空实例，
            # 卡死合并应用——与 GraphML「未标注概念跳过」同语义），warning 明示
            lossy = True
            warnings.append(f"实例 {name} 的类型未注册为概念，已丢弃（可先补概念定义后重新导入）")
            continue
        for p, o in g.predicate_objects(ind):
            pu, ou = str(p), str(o)
            if pu in (str(RDF.type), str(RDFS.label), str(RDFS.comment), str(SKOS.prefLabel), str(SKOS.altLabel), str(SKOS.definition)):
                continue
            if ou.startswith(str(RDF)) or ou.startswith(str(OWL)):
                continue
            if pu in seen_r:  # 对象属性 → 实例关系
                tgt = inst_by_uri.get(ou) or (str(o) and None)
                if tgt:
                    it["relations"].append({"rel": seen_r[pu], "target": tgt})
                else:
                    lossy = True
                    warnings.append(f"实例 {name} 关系 {seen_r[pu]} 的目标 {local(ou)} 不是已注册实例，丢弃")
            else:  # 其余断言 → attributes
                try:
                    val = o.toPython()
                    val = val.isoformat() if hasattr(val, "isoformat") else val
                except Exception:  # noqa: BLE001
                    val = ou
                it["attributes"][local(pu) or pu] = val
        if not it["attributes"]:
            it.pop("attributes")
        if not it["relations"]:
            it.pop("relations")
        spec["instances"].append(it)

    out = {"spec": spec, "warnings": warnings, "lossy": lossy,
           "note": "OWL/TTL/JSON-LD 有损导入（REQ-69 细则/REQ-235）：仅保留 类层次/SKOS 词表层次/对象属性/实例断言，datatype 断言落实例 attributes（固定策略一档）；公理、限制、推理语义丢弃（计入 warnings）"}
    json.dump(out, sys.stdout, ensure_ascii=False)
    return 0


# ---- 导出：spec_json → TTL ----

def esc(s) -> str:
    return str(s).replace("\\", "\\\\").replace('"', '\\"')


def export(argv) -> int:
    oid = ""
    fmt = TTL
    args = argv
    i = 0
    while i < len(args):
        if args[i] == "--ontology-id":
            oid = args[i + 1]; i += 2
        elif args[i] == "--format":
            fmt = args[i + 1]; i += 2
        else:
            i += 1
    spec = json.load(sys.stdin)
    base = f"urn:o:{sanitize(oid)}:"
    cu = lambda n: f"{base}concept:{sanitize(n)}"       # noqa: E731
    ru = lambda n: f"{base}relation:{sanitize(n)}"      # noqa: E731
    iu = lambda n: f"{base}instance:{sanitize(n)}"      # noqa: E731
    au = lambda k: f"{base}attr:{sanitize(k)}"          # noqa: E731

    L = []
    L.append(f'@prefix o: <{base}> .')
    L.append('@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .')
    L.append('@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .')
    L.append('@prefix owl: <http://www.w3.org/2002/07/owl#> .')
    L.append('@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .')
    L.append('')
    # REQ-216：空本体 spec（伴生绑定自动创建）concepts/relations/instances 可能为 null——
    # `or []` 容错（空 spec 导出为仅前缀的合法 TTL，宿主方案可正常 start/load）。
    for c in (spec.get("concepts") or []):
        u = cu(c["name"])
        L.append(f'<{u}> rdf:type owl:Class ; rdfs:label "{esc(c["name"])}"')
        if c.get("definition"):
            L[-1] += f' ;\n    rdfs:comment "{esc(c["definition"])}"'
        for p in c.get("parents", []):
            L[-1] += f' ;\n    rdfs:subClassOf <{cu(p)}>'
        L[-1] += ' .'
    for r in (spec.get("relations") or []):
        u = ru(r["name"])
        L.append(f'<{u}> rdf:type owl:ObjectProperty ; rdfs:label "{esc(r["name"])}"')
        if r.get("definition"):
            L[-1] += f' ;\n    rdfs:comment "{esc(r["definition"])}"'
        if r.get("from"):
            L[-1] += f' ;\n    rdfs:domain <{cu(r["from"])}>'
        if r.get("to"):
            L[-1] += f' ;\n    rdfs:range <{cu(r["to"])}>'
        L[-1] += ' .'
    for it in (spec.get("instances") or []):
        u = iu(it["name"])
        L.append(f'<{u}> rdf:type owl:NamedIndividual')
        if it.get("concept"):
            L[-1] += f' ;\n    rdf:type <{cu(it["concept"])}>'
        L[-1] += f' ;\n    rdfs:label "{esc(it["name"])}"'
        for k, v in (it.get("attributes") or {}).items():
            L[-1] += f' ;\n    <{au(k)}> "{esc(v)}"'
        for ir in it.get("relations", []):
            L[-1] += f' ;\n    <{ru(ir["rel"])}> <{iu(ir["target"])}>'
        L[-1] += ' .'

    ttl = "\n".join(L) + "\n"
    # rdflib 回读校验（导出即验证）
    try:
        g = Graph()
        g.parse(data=ttl, format=TTL)
    except Exception as e:  # noqa: BLE001
        sys.stderr.write(f"导出 TTL 校验失败: {e}\n")
        return 1
    sys.stdout.write(ttl)
    return 0


def main() -> int:
    if len(sys.argv) < 2:
        sys.stderr.write(__doc__)
        return 2
    cmd = sys.argv[1]
    if cmd == "parse":
        fmt = "turtle"
        if "--format" in sys.argv:
            fmt = sys.argv[sys.argv.index("--format") + 1]
        return parse(fmt)
    if cmd == "export":
        return export(sys.argv[2:])
    sys.stderr.write(f"未知命令 {cmd}\n")
    return 2


if __name__ == "__main__":
    sys.exit(main())
