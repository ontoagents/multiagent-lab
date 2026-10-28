#!/usr/bin/env python3
"""rdf-sidecar: OWL/RDF ↔ spec_json 双向转换（方案 04 v0.6 §2.2/§6，不自研 RDF 解析）。

用法:
  sidecar.py parse  --format owl_rdfxml|turtle|jsonld < input   -> stdout JSON {spec, warnings, lossy, note}
  sidecar.py export --ontology-id ID [--format turtle] < spec_json -> stdout TTL

导入映射（REQ-69 细则定稿，REQ-235/M62 扩 SKOS/JSON-LD/数据属性档；REQ-268/M77 数据属性声明捕获）:
  rdfs:label|skos:prefLabel（pref/alt 优先）→ name/label、rdfs:comment|skos:definition → definition、
  rdfs:subClassOf|skos:broader|skos:narrower(反向) → parents、
  owl:Class|rdfs:Class|skos:Concept → concept、
  owl:NamedIndividual+rdf:type → instance、对象属性断言 → 实例关系、
  datatype 属性断言 → attributes（实例级）；owl:DatatypeProperty 声明 → data_properties（REQ-268
  自「固定策略一档丢弃+warning」升档为**声明捕获**：name/label/definition/domain/range——
  domain 未注册为概念则留空不悬空，range xsd:* → 短名 string/number/integer/boolean/date；
  属性公理（函数性/限制等）仍丢弃 warning 明示）；其余公理/推理语义丢弃并计入 warnings。
导出 URN 规则与 pkg/ontology/spec 的 sanitize 逐字符一致（查询翻译共用）；
data_properties 声明导出为 <attr:名> owl:DatatypeProperty（与实例属性断言同 IRI，声明与使用统一）。
"""
import sys, json, decimal

# REQ-465：输出统一 UTF-8——Windows 非 UTF-8 运行时下 sys.stdout/stderr 编码随 ANSI 代码页
# （cp936 严格），非 GBK 字符会崩编码错误、JSON 输出成错码字节；固定 UTF-8 与导入侧
# 显式解码口径对齐（stdin 侧见 stdin_text），Go 侧按字节消费不再受运行环境漂移影响。
if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")

try:
    from rdflib import Graph, RDF, RDFS, OWL, URIRef, Literal, SKOS
except ImportError:
    sys.stderr.write("rdflib 未安装：pip install rdflib\n")
    sys.exit(3)


def stdin_text() -> tuple[str, list[str], bool]:
    """stdin 字节级读取+显式解码三级回落（REQ-465 Windows 导入编码根治）。

    sys.stdin.read() 的解码随运行环境漂移：Windows PYTHONUTF8/UTF-8 模式下 stdin 为
    surrogateescape——文件中的非 UTF-8 字节被读成孤立代理字符（\\udcXX），RDF/XML 在
    rdflib 严格 utf-8 编码处崩「surrogates not allowed」（真机实锤：64KB 文件单个 0x93
    残留字节）；cp936 严格环境则整读直接 UnicodeDecodeError。字节自读后按序回落：
    ①utf-8 严格（绝大多数文件，行为与 Linux 路径一致）；
    ②坏字节稀疏（≤max(4, 总量万分之一)）→ utf-8+replace 保正文——受损 UTF-8（典型如
      单个 cp1252 智能引号/GBK 尾字节残留），此形态走 GB18030 会把合法 UTF-8 中文整篇
      乱码，故优先保正文、坏字节落 U+FFFD 如实降级（lossy）；
    ③gb18030 严格（GBK/GB2312 超集）——全篇 GBK 中文文件无损解码（非 lossy）；
    ④兜底 utf-8+replace（非 UTF-8 亦非 GB18030 的脏数据，如实降级 lossy）。
    返回 (文本, warnings, 坏字节是否被替换)。
    """
    raw = sys.stdin.buffer.read()
    try:
        return raw.decode("utf-8"), [], False
    except UnicodeDecodeError:
        pass
    degraded = raw.decode("utf-8", errors="replace")
    n_bad = degraded.count("\ufffd")
    if n_bad <= max(4, len(raw) // 10000):
        pos = degraded.find("\ufffd")
        return degraded, [f"文件非纯 UTF-8：{n_bad} 个坏字节已替换为 U+FFFD（首个约在第 {pos} 字节），正文按 UTF-8 保留；建议修复源文件编码后重新导入"], True
    try:
        return raw.decode("gb18030"), ["文件非 UTF-8，已按 GB18030 解码（GBK/GB2312 超集，中文无损）；建议源文件转存 UTF-8"], False
    except UnicodeDecodeError:
        return degraded, [f"文件无法按 UTF-8/GB18030 解码：{n_bad} 个坏字节已替换为 U+FFFD"], True

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


def label_of(g, subj) -> str | None:
    # REQ-268/M77：展示标签独立于名称（label_or_local 在无标签时回落 local，不适合判断「有无标签」）
    for pred in (SKOS.prefLabel, SKOS.altLabel, RDFS.label):
        for lab in g.objects(subj, pred):
            s = str(lab).strip()
            if s:
                return s
    return None


XSD_SHORT = {
    "http://www.w3.org/2001/XMLSchema#string": "string",
    "http://www.w3.org/2001/XMLSchema#double": "number",
    "http://www.w3.org/2001/XMLSchema#float": "number",
    "http://www.w3.org/2001/XMLSchema#decimal": "number",
    "http://www.w3.org/2001/XMLSchema#integer": "integer",
    "http://www.w3.org/2001/XMLSchema#int": "integer",
    "http://www.w3.org/2001/XMLSchema#long": "integer",
    "http://www.w3.org/2001/XMLSchema#boolean": "boolean",
    "http://www.w3.org/2001/XMLSchema#date": "date",
    "http://www.w3.org/2001/XMLSchema#dateTime": "date",
}


def range_short(rng) -> str:
    # REQ-268/M77：rdfs:range → 短名；未知 xsd/自定义 IRI 诚实保留原始 IRI（导出侧按原样回写）
    if rng is None:
        return "string"
    s = str(rng)
    return XSD_SHORT.get(s, s)


def parse(format: str) -> int:
    content, dec_warnings, dec_lossy = stdin_text()
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

    warnings, lossy = list(dec_warnings), dec_lossy
    # REQ-328/B：余量全量账本——凡未投影且非「纯系统词汇」三元组全部 N-Triples 入账（无上限，
    # 规模由 IMPORT_MAX_BODY_MB 上界自然约束），导出拼接还原；对账恒等式
    #   triples_in = vocabulary（纯系统）+ projected（投影消费）+ remainder（余量）
    # unaccounted = 0 结构性保证（三集合按构造划分）；对账块随导入报告透出。
    unprojected = []
    consumed = set()  # 投影消费的三元组（逐点 mark；漏标落余量为无害重复——余量按图重解析去重）

    def mark(*triples):
        for t in triples:
            consumed.add(t)

    spec = {"name": "", "description": "", "concepts": [], "relations": [], "instances": []}

    # ---- REQ-328/A：用法推断补声明（deterministic 零 LLM；报告 inferred 明示非文件原生）----
    # 纯 ABox 形态（77 号触发案例：类型只被使用未声明、个体无 owl:NamedIndividual 标记）投影全漏——
    # 三类推断补齐：①类型使用→概念声明 ②类型使用主体→实例 ③实例断言谓词→属性声明。
    XSD_NS = "http://www.w3.org/2001/XMLSchema#"
    SYS_NS = (str(RDF), str(RDFS), str(OWL), str(SKOS), XSD_NS)

    def is_sys(u) -> bool:
        return isinstance(u, URIRef) and any(str(u).startswith(ns) for ns in SYS_NS)

    def purely_system(t) -> bool:
        s, p, o = t
        return is_sys(s) and is_sys(p) and is_sys(o)

    declared_classes = set(g.subjects(RDF.type, OWL.Class)) | set(g.subjects(RDF.type, RDFS.Class)) | set(g.subjects(RDF.type, SKOS.Concept))
    inferred_notes = []
    used_as_class = set()
    for s, o in g.subject_objects(RDF.type):
        if isinstance(o, URIRef) and not is_sys(o) and isinstance(s, URIRef) and not is_sys(s):
            used_as_class.add(o)  # o 被用作类型 → 推断为概念
    for s, o in g.subject_objects(RDFS.subClassOf):
        for u in (s, o):
            if isinstance(u, URIRef) and not is_sys(u):
                used_as_class.add(u)  # subClassOf 两端皆类
    for s, o in g.subject_objects(RDFS.domain):
        if isinstance(o, URIRef) and not is_sys(o):
            used_as_class.add(o)  # domain/range 宾语为类（主语是属性本体，不推断）
    for s, o in g.subject_objects(RDFS.range):
        if isinstance(o, URIRef) and not is_sys(o):
            used_as_class.add(o)
    inferred_classes = {c for c in used_as_class if c not in declared_classes}
    classes = declared_classes | inferred_classes
    if inferred_classes:
        inferred_notes.append(f"概念声明（用法推断补齐，非文件原生）: {len(inferred_classes)} 个")

    # ---- 类：owl:Class / rdfs:Class / skos:Concept（REQ-235① SKOS 词表；跳过系统内建类）----
    seen_c = {}
    for c in sorted(classes, key=str):  # 确定性输出（set 迭代序随机）
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
        for t in g.objects(c, RDF.type):
            if isinstance(t, URIRef) and str(t) in (str(OWL.Class), str(RDFS.Class), str(SKOS.Concept)):
                mark((c, RDF.type, t))
        for lab_pred in (SKOS.prefLabel, SKOS.altLabel, RDFS.label):
            for lab in g.objects(c, lab_pred):
                mark((c, lab_pred, lab))
        comment = definition_of(g, c)
        if comment:
            concept["definition"] = comment
        for cm_pred in (RDFS.comment, SKOS.definition):
            for cm in g.objects(c, cm_pred):
                mark((c, cm_pred, cm))
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
                # REQ-328/B：匿名父类结构落余量账本（BNode 链全域由余量清扫承载，此处不重复收集）
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
                    mark((c, RDFS.subClassOf, p))
        # skos:broader → parents；skos:narrower 反向 → parents（目标须已注册为概念，外部引用 warning 丢弃）
        for p in list(g.objects(c, SKOS.broader)) + list(g.subjects(SKOS.narrower, c)):
            if not isinstance(p, URIRef):
                continue
            pname = name_by_uri.get(str(p))
            if pname and pname != mine["name"] and pname not in parents:
                parents.append(pname)
                mark((c, SKOS.broader, p))
            elif pname is None and not parents:
                lossy = True
                warnings.append(f"{mine['name']} 的 skos:broader/narrower 目标 {local(str(p))} 未注册为概念，丢弃")
        if parents:
            mine["parents"] = parents

    # ---- 对象属性 ----
    seen_r = {}
    props = set(g.subjects(RDF.type, OWL.ObjectProperty))
    for pr in sorted(props, key=str):  # 确定性输出
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
        rel = {"name": name, "evidence": "declared", "_uri": uri}  # REQ-295①：显式 OWL 声明=declared 档（_uri 供 REQ-473 特性捕获索引，出 spec 前剥离）
        mark((pr, RDF.type, OWL.ObjectProperty))
        for lab in g.objects(pr, RDFS.label):
            mark((pr, RDFS.label, lab))
        comment = g.value(pr, RDFS.comment)
        if comment:
            rel["definition"] = str(comment)
            mark((pr, RDFS.comment, comment))
        dom = g.value(pr, RDFS.domain)
        rng = g.value(pr, RDFS.range)
        if dom is not None:
            mark((pr, RDFS.domain, dom))
        if rng is not None:
            mark((pr, RDFS.range, rng))
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
    # REQ-268/M77（D-O22 批次二）：owl:DatatypeProperty 声明捕获入模型——自 REQ-235⑥「丢弃+warning」升档。
    # name=label 优先（与概念/关系同 label_or_local 口径）；domain 未注册为概念则留空（不产生悬空引用）；
    # range xsd:* → 短名，未知保留 IRI；函数性/限制等属性公理仍丢弃 warning 明示。
    # 声明与实例 attributes 键同名关联：实例断言的谓词若命中声明 URI，键归一为声明名（label 可能≠URI local）。
    dt_props = set(g.subjects(RDF.type, OWL.DatatypeProperty))
    data_properties = []
    dp_name_by_uri = {}
    seen_dp = set()
    for pr in sorted(dt_props, key=str):  # 确定性输出
        if not isinstance(pr, URIRef) or str(pr).startswith((str(RDF), str(RDFS), str(OWL), str(SKOS))):
            continue
        name = label_or_local(g, pr)
        if name in seen_dp:
            warnings.append(f"重复数据属性名 {name} 已跳过")
            continue
        seen_dp.add(name)
        dp_name_by_uri[str(pr)] = name
        dp = {"name": name}
        mark((pr, RDF.type, OWL.DatatypeProperty))
        lbl = label_of(g, pr)
        if lbl and lbl != name:
            dp["label"] = lbl
        comment = definition_of(g, pr)
        if comment:
            dp["definition"] = comment
        dom = g.value(pr, RDFS.domain)
        if dom is not None:
            mark((pr, RDFS.domain, dom))
            if str(dom) in seen_c:
                dp["domain"] = seen_c[str(dom)]
            else:
                warnings.append(f"数据属性 {name} 定义域 {label_or_local(g, dom)} 未注册为概念，留空")
        rng_val = g.value(pr, RDFS.range)
        if rng_val is not None:
            mark((pr, RDFS.range, rng_val))
        dp["range"] = range_short(rng_val)
        for lab in g.objects(pr, RDFS.label):
            mark((pr, RDFS.label, lab))
        data_properties.append(dp)
    if data_properties:
        spec["data_properties"] = data_properties

    # REQ-473/M273：对象属性特性捕获（transitive/symmetric/functional/inverse_functional +
    # owl:inverseOf）——升级为投影进 Relation（原落余量）；双端均注册为关系才入层。
    _CH_TYPES = {
        OWL.TransitiveProperty: "transitive",
        OWL.SymmetricProperty: "symmetric",
        OWL.FunctionalProperty: "functional",
        OWL.InverseFunctionalProperty: "inverse_functional",
    }
    rel_uri_index = {r["_uri"]: r for r in spec["relations"] if r.get("_uri")}
    for (pr, pt, po) in g.triples((None, RDF.type, None)):
        ch = _CH_TYPES.get(po)
        if ch is None or not isinstance(pr, URIRef):
            continue
        r = rel_uri_index.get(str(pr))
        if r is not None:
            r.setdefault("characteristics", [])
            if ch not in r["characteristics"]:
                r["characteristics"].append(ch)
            mark((pr, pt, po))
    for (pr, pt, po) in g.triples((None, OWL.inverseOf, None)):
        if not isinstance(pr, URIRef) or not isinstance(po, URIRef):
            continue
        r = rel_uri_index.get(str(pr))
        tgt = rel_uri_index.get(str(po))
        if r is not None and tgt is not None:
            r["inverse_of"] = tgt["name"]
            mark((pr, pt, po))
    for r in spec["relations"]:
        r.pop("_uri", None)

    # REQ-328/B：属性链等其余属性公理不再特殊收集——余量全域清扫（见尾部 reconciliation 段）
    # 会原样入账（含 BNode 结构），导出拼接还原

    # REQ-269/M78（D-O22 批次三）：类级公理捕获——owl:disjointWith / owl:equivalentClass。
    # 双端均已注册为概念才入 axioms（URI → 已注册概念名归一）；任一端未注册则 warning 丢弃
    # （防悬空引用，沿 SKOS broader/narrower 先例）；同 (type, subject) 多目标合并为一条。
    axioms = []
    ax_index = {}
    for pred, atype in ((OWL.disjointWith, "disjoint_with"), (OWL.equivalentClass, "equivalent_class")):
        for subj, obj in g.subject_objects(pred):
            if not isinstance(subj, URIRef) or not isinstance(obj, URIRef):
                continue  # 匿名/空白节点公理（限制类）仍丢弃 lossy，warning 于类遍历段已有口径
            s_name = seen_c.get(str(subj))
            o_name = seen_c.get(str(obj))
            if s_name is None or o_name is None:
                miss = s_name if s_name is None else o_name
                warnings.append(f"公理 {atype} 端点 {label_or_local(g, subj if s_name is None else obj)} 未注册为概念，丢弃")
                lossy = True
                _ = miss
                continue  # 余量全域清扫承载
            if s_name == o_name:
                continue
            mark((subj, pred, obj))
            key = (atype, s_name)
            if key in ax_index:
                if o_name not in axioms[ax_index[key]]["targets"]:
                    axioms[ax_index[key]]["targets"].append(o_name)
            else:
                ax_index[key] = len(axioms)
                axioms.append({"type": atype, "subject": s_name, "targets": [o_name]})
    if axioms:
        spec["axioms"] = axioms

    # ---- 实例（owl:NamedIndividual 标记 ∪ REQ-328/A 类型使用推断——无标记个体补齐）----
    declared_props_uris = props | dt_props
    inst_subjects = {i for i in g.subjects(RDF.type, OWL.NamedIndividual) if isinstance(i, URIRef) and not is_sys(i)}
    for s, o in g.subject_objects(RDF.type):
        if (isinstance(s, URIRef) and isinstance(o, URIRef) and not is_sys(s)
                and str(o) in seen_c and str(s) not in seen_c and s not in declared_props_uris):
            inst_subjects.add(s)  # 主体被 rdf:type 挂到已注册概念=个体（无 NamedIndividual 标记，推断补齐）
    if len(inst_subjects) > len(set(g.subjects(RDF.type, OWL.NamedIndividual))):
        inferred_notes.append(f"实例（类型使用推断补齐，非文件原生标记）: 新增 {len(inst_subjects) - len([i for i in g.subjects(RDF.type, OWL.NamedIndividual) if isinstance(i, URIRef)])} 个")
    inst_by_uri = {}
    for ind in sorted(inst_subjects, key=str):  # 确定性输出
        if not isinstance(ind, URIRef):
            continue
        uri = str(ind)
        name = label_or_local(g, ind)
        if name in inst_by_uri:
            warnings.append(f"重复实例名 {name} 已跳过")
            continue
        inst_by_uri[uri] = name
        it = {"name": name, "attributes": {}, "relations": []}
        for t in g.objects(ind, RDF.type):
            mark((ind, RDF.type, t))
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
                    mark((ind, p, o))
                else:
                    lossy = True
                    warnings.append(f"实例 {name} 关系 {seen_r[pu]} 的目标 {local(ou)} 不是已注册实例，丢弃")
            else:  # 其余断言 → attributes（REQ-268/M77：谓词命中数据属性声明时键归一为声明名）
                mark((ind, p, o))
                try:
                    val = o.toPython()
                    # REQ-328/E：xsd:decimal → Python Decimal 不可 JSON 序列化（json.dump 崩溃 exit=1
                    # sidecar 整体失败）——转 float；datetime 等有 isoformat 的转 ISO 串
                    if isinstance(val, decimal.Decimal):
                        val = float(val)
                    else:
                        val = val.isoformat() if hasattr(val, "isoformat") else val
                except Exception:  # noqa: BLE001
                    val = ou
                it["attributes"][dp_name_by_uri.get(pu) or local(pu) or pu] = val
        if not it["attributes"]:
            it.pop("attributes")
        if not it["relations"]:
            it.pop("relations")
        spec["instances"].append(it)

    # ---- REQ-328/A③：属性声明推断（实例断言谓词未声明→补声明；域值域自使用统计）----
    inst_uris = set(inst_by_uri.keys())
    used_preds = {}
    for ind in inst_subjects:
        types = [str(t) for t in g.objects(ind, RDF.type) if isinstance(t, URIRef) and str(t) in seen_c]  # inst_subjects 元素为 URIRef 原词元，可直接查图
        for p, o in g.predicate_objects(ind):
            pu, ou = str(p), str(o)
            if pu in (str(RDF.type), str(RDFS.label), str(RDFS.comment), str(SKOS.prefLabel), str(SKOS.altLabel), str(SKOS.definition)):
                continue
            if is_sys(p):
                continue
            used_preds.setdefault(pu, {"subjects": set(), "objects": set(), "uri_objects": 0, "total": 0})
            used_preds[pu]["subjects"].add(tuple(types[:1]))
            used_preds[pu]["total"] += 1
            if isinstance(o, URIRef):
                used_preds[pu]["uri_objects"] += 1
    inferred_obj, inferred_dp = 0, 0
    for pu, stat in used_preds.items():
        if pu in seen_r or pu in dp_name_by_uri:
            continue
        # 声明名=URI local；域=主体类型众数；对象属性值域=目标实例类型众数 / 数据属性值域=字面量 datatype 短名
        dom_name = _mode_concept(stat["subjects"], seen_c)
        if stat["uri_objects"] > 0:
            rng_name = _mode_concept(_inst_types(g, inst_by_uri, pu, seen_c), seen_c)
            if not dom_name and not rng_name:
                continue
            spec["relations"].append({"name": local(pu) or pu, "from": dom_name or "", "to": rng_name or "",
                                      "evidence": "values", "definition": "（声明缺失，导入时按用法推断补齐）"})
            inferred_obj += 1
        else:
            spec.setdefault("data_properties", []).append({"name": local(pu) or pu, "domain": dom_name or "",
                                                           "range": "string", "definition": "（声明缺失，导入时按用法推断补齐）"})
            inferred_dp += 1
    if inferred_obj:
        inferred_notes.append(f"对象关系声明（用法推断补齐）: {inferred_obj} 条")
    if inferred_dp:
        inferred_notes.append(f"数据属性声明（用法推断补齐）: {inferred_dp} 条")

    # ---- REQ-328/B：余量全域清扫 + 对账恒等式 ----
    # V=纯系统词汇三元组（主谓宾全在 rdf/rdfs/owl/skos/xsd 命名空间）；C=投影消费（consumed 逐点标记，
    # 漏标落余量为无害重复——余量按图重解析同三元组去重）；R=其余全部（含 BNode 限制结构/头部元数据/
    # 自定义谓词非实例断言）原样 N-Triples 入账无上限。恒等式 unaccounted=0 按构造成立。
    vocabulary_n, remainder = 0, []
    for t in g:
        if t in consumed or purely_system(t):
            vocabulary_n += 1 if purely_system(t) and t not in consumed else 0
            if t in consumed and not purely_system(t):
                pass
            continue
        s_, p_, o_ = t
        remainder.append(f"{s_.n3()} {p_.n3()} {o_.n3()} .")
    triples_in = len(g)
    projected_n = len([t for t in consumed if not purely_system(t)])
    reconciliation = {
        "triples_in": triples_in,
        "vocabulary": vocabulary_n,
        "projected": projected_n,
        "remainder": len(remainder),
        "inferred_notes": inferred_notes,
        "unaccounted": triples_in - vocabulary_n - projected_n - len(remainder),
    }
    semantic = max(1, triples_in - vocabulary_n)
    capture_rate = round(min(1.0, (projected_n + inferred_obj + inferred_dp) / semantic), 4)
    reconciliation["capture_rate"] = capture_rate
    if capture_rate < 0.5 and triples_in > 0:
        warnings.append(f"捕获率 {capture_rate:.0%} 低于 50%——大部分内容未被投影为可编辑结构（余量账本已保全部原样，导出与运行可还原；建议检查文件形态或人工补建结构）")

    unprojected = remainder  # REQ-328/B：axioms-raw 升级为全量余量账本（字段名沿用导出拼接链路）
    out = {"spec": spec, "warnings": warnings, "lossy": lossy,
           "note": "OWL/TTL/JSON-LD 有损导入（REQ-69 细则/REQ-235；REQ-268 数据属性声明捕获；REQ-328 用法推断补声明+全量余量账本）：保留 类层次/SKOS 词表层次/对象属性/数据属性声明/实例断言，datatype 断言落实例 attributes；未投影内容全量入余量账本 unprojected 透传（对账恒等式见 reconciliation）",
           "unprojected": unprojected, "reconciliation": reconciliation}
    json.dump(out, sys.stdout, ensure_ascii=False, default=str)
    return 0


def _mode_concept(type_tuples, seen_c):
    """主体/目标类型众数→概念名（types 为 (uri,) 元组集合）。"""
    from collections import Counter
    c = Counter(t[0] for t in type_tuples if t and t[0])
    for uri, _ in c.most_common(1):
        return seen_c.get(uri, "")
    return ""


def _inst_types(g, inst_by_uri, pu, seen_c):
    """谓词 pu 的目标实例的类型集合（对象属性值域推断用）。
    inst_by_uri 键为 str(ind)——rdflib 词元匹配需 URIRef 型，回查一律包装。"""
    out = set()
    for ind, name in inst_by_uri.items():
        for p, o in g.predicate_objects(URIRef(ind)):
            if str(p) == pu and isinstance(o, URIRef) and str(o) in inst_by_uri:
                for t in g.objects(o, RDF.type):
                    if isinstance(t, URIRef) and str(t) in seen_c:
                        out.add((str(t),))
    return out


# ---- 导出：spec_json → TTL ----

def esc(s) -> str:
    return str(s).replace("\\", "\\\\").replace('"', '\\"')


def typed_literal(v, rng):
    """REQ-474/M274：按 range 类型化字面量（与 Go typedLiteral 对拍一致）。"""
    if rng == "integer":
        if isinstance(v, bool):
            pass
        elif isinstance(v, (int, float)) and float(v).is_integer():
            return str(int(v))
        elif isinstance(v, str):
            try:
                int(v.strip())
                return v.strip()
            except ValueError:
                pass
    elif rng == "number":
        if isinstance(v, bool):
            pass
        elif isinstance(v, (int, float)):
            s = repr(float(v)) if isinstance(v, float) else str(v)
            if "." not in s and "e" not in s and "E" not in s:
                s += ".0"
            return s
    elif rng == "boolean":
        if isinstance(v, bool):
            return "true" if v else "false"
        if isinstance(v, str):
            if v.strip() in ("true", "True", "1"):
                return "true"
            if v.strip() in ("false", "False", "0"):
                return "false"
    elif rng == "date":
        return f'"{esc(v)}"^^xsd:date'
    return f'"{esc(v)}"'


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
    # REQ-465：spec_json 按字节读入固定 UTF-8 解码（sys.stdin 文本读随运行环境漂移，同 stdin_text 口径；
    # 平台侧生成的 spec_json 恒为合法 UTF-8，无需回落链）
    spec = json.loads(sys.stdin.buffer.read().decode("utf-8"))
    base = f"urn:o:{sanitize(oid)}:"
    cu = lambda n: f"{base}concept:{sanitize(n)}"       # noqa: E731
    ru = lambda n: f"{base}relation:{sanitize(n)}"      # noqa: E731
    iu = lambda n: f"{base}instance:{sanitize(n)}"      # noqa: E731
    au = lambda k: f"{base}attr:{sanitize(k)}"          # noqa: E731

    # REQ-268/M77：数据属性 range 短名 → xsd IRI（未知形态：http(s) IRI 原样 <尖括号>，其余回落 xsd:string）
    XSD_IRI = {
        "string": "xsd:string", "number": "xsd:double", "integer": "xsd:integer",
        "boolean": "xsd:boolean", "date": "xsd:date",
    }

    def range_iri(rng) -> str:
        r = (rng or "string").strip()
        if r in XSD_IRI:
            return XSD_IRI[r]
        if r.startswith("http://") or r.startswith("https://"):
            return f"<{r}>"
        return "xsd:string"

    L = []
    L.append(f'@prefix o: <{base}> .')
    L.append('@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .')
    L.append('@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .')
    L.append('@prefix owl: <http://www.w3.org/2002/07/owl#> .')
    L.append('@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .')
    L.append('')
    # REQ-474/M274：owl:Ontology 合成头（与 Go 导出器对拍口径一致——空名不出头，保空 spec 不变量）
    if spec.get("name"):
        _ont_head = f'<{base}__ontology> rdf:type owl:Ontology ; rdfs:label "{esc(spec["name"])}"'
        if spec.get("description"):
            _ont_head += f' ;\n    rdfs:comment "{esc(spec["description"])}"'
        L.append(_ont_head + ' .')
    # REQ-216：空本体 spec（伴生绑定自动创建）concepts/relations/instances 可能为 null——
    # `or []` 容错（空 spec 导出为仅前缀的合法 TTL，宿主方案可正常 start/load）。
    for c in (spec.get("concepts") or []):
        u = cu(c["name"])
        L.append(f'<{u}> rdf:type owl:Class ; rdfs:label "{esc(c["name"])}"')
        if c.get("definition"):
            L[-1] += f' ;\n    rdfs:comment "{esc(c["definition"])}"'
        # REQ-474/M274：展示名 rdfs:label 导出（同值不重复出——与 Go 导出器对拍口径一致）
        if c.get("label") and c["label"] != c["name"]:
            L[-1] += f' ;\n    rdfs:label "{esc(c["label"])}"'
        for p in c.get("parents", []):
            L[-1] += f' ;\n    rdfs:subClassOf <{cu(p)}>'
        L[-1] += ' .'
    for r in (spec.get("relations") or []):
        u = ru(r["name"])
        L.append(f'<{u}> rdf:type owl:ObjectProperty ; rdfs:label "{esc(r["name"])}"')
        if r.get("definition"):
            L[-1] += f' ;\n    rdfs:comment "{esc(r["definition"])}"'
        if r.get("label") and r["label"] != r["name"]:
            L[-1] += f' ;\n    rdfs:label "{esc(r["label"])}"'
        if r.get("from"):
            L[-1] += f' ;\n    rdfs:domain <{cu(r["from"])}>'
        if r.get("to"):
            L[-1] += f' ;\n    rdfs:range <{cu(r["to"])}>'
        # REQ-473/M273：特性附加 rdf:type 行 + owl:inverseOf（与 Go 导出器 ttlexport.go 对拍口径一致）
        _CH_IRI = {
            "transitive": "owl:TransitiveProperty",
            "symmetric": "owl:SymmetricProperty",
            "functional": "owl:FunctionalProperty",
            "inverse_functional": "owl:InverseFunctionalProperty",
        }
        for ch in (r.get("characteristics") or []):
            iri = _CH_IRI.get(ch)
            if iri:
                L[-1] += f' ;\n    rdf:type {iri}'
        if r.get("inverse_of"):
            L[-1] += f' ;\n    owl:inverseOf <{ru(r["inverse_of"])}>'
        L[-1] += ' .'
    for dp in (spec.get("data_properties") or []):
        # REQ-268/M77：声明导出为 <attr:名> owl:DatatypeProperty——与实例属性断言同 IRI（声明与使用统一）
        u = au(dp["name"])
        L.append(f'<{u}> rdf:type owl:DatatypeProperty ; rdfs:label "{esc(dp["name"])}"')
        if dp.get("definition"):
            L[-1] += f' ;\n    rdfs:comment "{esc(dp["definition"])}"'
        if dp.get("label") and dp["label"] != dp["name"]:
            L[-1] += f' ;\n    rdfs:label "{esc(dp["label"])}"'
        if dp.get("domain"):
            L[-1] += f' ;\n    rdfs:domain <{cu(dp["domain"])}>'
        L[-1] += f' ;\n    rdfs:range {range_iri(dp.get("range"))}'
        L[-1] += ' .'
    # REQ-269/M78：公理导出回写——<概念> owl:disjointWith / owl:equivalentClass <目标>（概念同 IRI 口径）
    for ax in (spec.get("axioms") or []):
        pred = "owl:disjointWith" if ax.get("type") == "disjoint_with" else "owl:equivalentClass"
        tgts = ", ".join(f"<{cu(t)}>" for t in (ax.get("targets") or []))
        if tgts:
            L.append(f'<{cu(ax["subject"])}> {pred} {tgts} .')
    for it in (spec.get("instances") or []):
        u = iu(it["name"])
        L.append(f'<{u}> rdf:type owl:NamedIndividual')
        if it.get("concept"):
            L[-1] += f' ;\n    rdf:type <{cu(it["concept"])}>'
        L[-1] += f' ;\n    rdfs:label "{esc(it["name"])}"'
        # 键序统一 sorted（与 Go 导出器 ttlexport.go 对拍口径一致：消除 dict 插入序不确定）
        # REQ-474/M274：按数据属性声明 range 类型化（与 Go typedLiteral 对拍一致）
        dp_ranges = {d["name"]: d.get("range") or "" for d in (spec.get("data_properties") or [])}
        for k, v in sorted((it.get("attributes") or {}).items()):
            L[-1] += f' ;\n    <{au(k)}> {typed_literal(v, dp_ranges.get(k, ""))}'
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
    # REQ-293①：三元组精确计数标记（stderr，不污染 stdout 的 TTL 内容）——运行平面装载计数硬闸对账数据源
    sys.stderr.write(f"TRIPLES {len(g)}\n")
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
    if cmd == "canon":
        # REQ-328/C 运行装载双源：original 全图 canonical Turtle 直出（rdflib 图同构 100% 无损），
        # stderr TRIPLES 计数与 export 同口径（运行平面装载计数硬闸对账数据源）
        fmt = "turtle"
        if "--format" in sys.argv:
            fmt = sys.argv[sys.argv.index("--format") + 1]
        return canon(fmt)
    if cmd == "reason":
        # REQ-255②/M62 批次（60 号 H3）：推理级一致性检查档（owlrl OWL 2 RL 闭包；
        # 完整 DL 推理 HermiT 不引入——学习定位内 owlrl 够用）。输入 TTL，输出 JSON。
        fmt = "turtle"
        if "--format" in sys.argv:
            fmt = sys.argv[sys.argv.index("--format") + 1]
        return reason(fmt)
    sys.stderr.write(f"未知命令 {cmd}\n")
    return 2


def canon(format: str) -> int:
    content, dec_warnings, _ = stdin_text()
    for w in dec_warnings:
        sys.stderr.write(f"note: {w}\n")
    g = Graph()
    fmt = {"owl_rdfxml": RDFXML, "turtle": TTL, "jsonld": JSONLD}.get(format)
    if fmt is None:
        sys.stderr.write(f"未知 canon 格式 {format}\n")
        return 2
    try:
        g.parse(data=content, format=fmt)
    except Exception as e:  # noqa: BLE001
        sys.stderr.write(f"RDF 解析失败: {e}\n")
        return 1
    ttl = g.serialize(format=TTL)
    try:
        g2 = Graph()
        g2.parse(data=ttl, format=TTL)
    except Exception as e:  # noqa: BLE001
        sys.stderr.write(f"canonical TTL 回读校验失败: {e}\n")
        return 1
    sys.stderr.write(f"TRIPLES {len(g2)}\n")
    sys.stdout.write(ttl)
    return 0


def reason(format: str) -> int:
    content, dec_warnings, _ = stdin_text()
    for w in dec_warnings:
        sys.stderr.write(f"note: {w}\n")
    g = Graph()
    fmt = {"owl_rdfxml": RDFXML, "turtle": TTL, "jsonld": JSONLD}.get(format)
    if fmt is None:
        sys.stderr.write(f"未知推理检查格式 {format}\n")
        return 2
    try:
        g.parse(data=content, format=fmt)
    except Exception as e:  # noqa: BLE001
        sys.stderr.write(f"RDF 解析失败: {e}\n")
        return 1
    try:
        import owlrl  # 推理检查档依赖（REQ-255②；pip install owlrl）
        from owlrl.AxiomaticTriples import OWLRL_Axiomatic_Triples, OWLRL_D_Axiomatic_Triples
        sem = owlrl.OWLRL_Extension(g, OWLRL_Axiomatic_Triples, OWLRL_D_Axiomatic_Triples, rdfs=True)
        sem.closure()
        violations = [str(m) for m in sem.error_messages]
        consistent = len(violations) == 0
    except ImportError:
        sys.stderr.write("owlrl 未安装：pip install owlrl（推理检查档依赖）\n")
        return 3
    except Exception as e:  # noqa: BLE001
        # 推理器内部异常不伪装成「一致」——如实报 error 由调用方降级处理
        json.dump({"consistent": None, "violations": [], "error": f"owlrl 推理失败: {e}",
                   "note": "OWL 2 RL 闭包一致性检查（owlrl；HermiT 完整 DL 推理不引入）"}, sys.stdout, ensure_ascii=False)
        return 0
    json.dump({"consistent": consistent, "violations": violations[:50],
               "note": "OWL 2 RL 闭包一致性检查（owlrl；HermiT 完整 DL 推理不引入，60 号 H3 口径）"}, sys.stdout, ensure_ascii=False)
    return 0


if __name__ == "__main__":
    sys.exit(main())
